#!/usr/bin/env bash
# Runs on a cell. From inside a running sandbox, it tries to open a connection to what an agent must not reach and to the one thing
# it must: the model proxy. It passes when the proxy is open and nothing else is. A probe that opens nothing proves nothing, so the
# proxy being closed is a failure too.
#
# SANDBOX names the sandbox, and is the first ready one when not named. FRONT_DOOR is the front door's private address. Both
# openshell and the address of the way out can be replaced, which is how a test runs it.
set -uo pipefail
export PATH=/opt/openshell/bin:/usr/local/bin:/usr/bin:/bin:$PATH XDG_CONFIG_HOME=${XDG_CONFIG_HOME:-/etc/openshell-xdg} XDG_STATE_HOME=${XDG_STATE_HOME:-/var/lib/openshell-xdg}
export OPENSHELL_LOCAL_TLS_DIR=${OPENSHELL_LOCAL_TLS_DIR:-$XDG_STATE_HOME/openshell/tls}
sandbox=${SANDBOX:-$(openshell sandbox list 2>/dev/null | awk 'NR>1 && $NF=="Ready" {print $1; exit}')}
[ -n "$sandbox" ] || { echo "no sandbox is running: start an agent in a demo first" >&2; exit 2; }

inside() { openshell sandbox exec --name "$sandbox" --no-tty -- "$@" < /dev/null; }
# The one thing a sandbox may reach is the model proxy, which its own policy names. A command run in a sandbox has a bare
# environment and the agent's process is gone between turns, so the address is read from the policy.
policy=$(openshell sandbox get "$sandbox" 2>/dev/null)
rules=$(awk '/^  network_policies:/ {on=1; next} on && /^  [a-z]/ {on=0} on && /^    [a-z_0-9]+:$/ {gsub(/[ :]/, ""); print}' <<<"$policy")
way_host=$(awk '/^    gusto_model_proxy:/ {on=1} on && /host:/ {print $NF; exit}' <<<"$policy")
way_port=$(awk '/^    gusto_model_proxy:/ {on=1} on && /port:/ {print $NF; exit}' <<<"$policy")
way=${MODEL_ADDRESS:-$way_host:$way_port}
[[ "$way" == *:* && "$way" != ":" ]] || { echo "the sandbox's policy names no model proxy to test against" >&2; exit 2; }
way_host=${way%:*}; way_port=${way#*:}

mine=${PRIVATE_IP:-$(hostname -I | awk '{print $1}')}
efs=${EFS_ADDRESS:-$(mount | sed -nE 's/.*type nfs4.*addr=([0-9.]+).*/\1/p' | head -1)}
# The machine's own file names a local stub, and the resolver behind it is the one an agent must not reach.
resolver=${RESOLVER:-$(awk '/^nameserver/ {print $2; exit}' /run/systemd/resolve/resolv.conf 2>/dev/null)}
targets=(
  "$way_host:$way_port"
  169.254.169.254:80 169.254.169.254:443
  "$mine:22" "$mine:80" "$mine:443" "$mine:8041" "$mine:8060"
  "$way_host:22" "$way_host:80" "$way_host:8041" "$way_host:8060" "$way_host:$((way_port - 1))" "$way_host:$((way_port + 1))"
  ${resolver:+"$resolver:53"}
  example.com:443 1.1.1.1:443 8.8.8.8:53
)
[ -z "$efs" ] || targets+=("$efs:2049")
[ -z "${FRONT_DOOR:-}" ] || targets+=("$FRONT_DOOR:22" "$FRONT_DOOR:80" "$FRONT_DOOR:443" "$FRONT_DOOR:8050" "$FRONT_DOOR:8090")

probe='for t in "$@"; do h=${t%:*}; p=${t#*:}; if timeout 4 bash -c "exec 3<>/dev/tcp/$h/$p" 2>/dev/null; then echo "OPEN $t"; else echo "closed $t"; fi; done'
result=$(inside bash -c "$probe" _ "${targets[@]}")
printf '%s\n' "$result"

# The sandbox answers names itself, with addresses that lead nowhere until the proxy is asked, so a name an agent makes up does not
# leave the sandbox as a lookup. An address from the real internet means the sandbox's names are resolved outside it.
made_up="isolation-$RANDOM$RANDOM.example.com"
answer=$(inside getent hosts "$made_up" | awk '{print $1; exit}')

failed=0
case "$answer" in
  198.18.* | 198.19.*) ;;
  *) echo "FAIL: a name that does not exist resolved to '${answer:-nothing}' inside the sandbox, which is not the sandbox's own answer, so its lookups may leave it" >&2; failed=1 ;;
esac
if others=$(grep -vx gusto_model_proxy <<<"$rules"); [ -n "$others" ]; then
  echo "FAIL: the sandbox's policy allows more than the model proxy: $(tr '\n' ' ' <<<"$others")" >&2
  failed=1
fi
if ! grep -qx "OPEN $way_host:$way_port" <<<"$result"; then
  echo "FAIL: the model proxy at $way is not open from the sandbox, so this probe cannot tell blocked from broken" >&2
  failed=1
fi
if unexpected=$(grep '^OPEN ' <<<"$result" | grep -vx "OPEN $way_host:$way_port" | grep -vE '^OPEN 127\.'); [ -n "$unexpected" ]; then
  echo "FAIL: a sandbox can open:" >&2
  echo "$unexpected" >&2
  failed=1
fi
[ "$failed" -eq 0 ] && echo "PASS: from inside a sandbox only the model proxy ($way) is open, of $(grep -c . <<<"$result") addresses tried, and its names are answered inside it"
exit "$failed"
