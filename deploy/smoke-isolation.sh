#!/bin/sh
# One job whose harness probes its own box, and a check of what it found. The harness
# is a fixture that runs a script as an agent would run a command: it tries to reach
# what a job must not reach, and prints what it sees. Run it as deploy/smoke.sh is run,
# inside the server:
#
#   docker compose exec -T server sh < ../smoke-isolation.sh
#
# This is the container isolation of an install. It passes when the box has no network
# interface but loopback, opens no connection to the places below, sees no container
# runtime socket and none of the server's data, and gains no privilege. It says nothing
# about a kernel bug: a container shares the host's kernel.
set -eu

data=$(dirname "$GUSTO_JOBS")
smoke="$data/smoke"
rm -rf "$smoke/isolation-job"
mkdir -p "$smoke/isolation-repo" "$smoke/isolation-harnesses"

cd "$smoke/isolation-repo"
if [ ! -d .git ]; then
  echo "nothing to change" > README
  git init -q . && git add -A
  git -c user.email=gusto@localhost -c user.name=gusto commit -qm "a repository to probe from"
fi

cat > "$smoke/isolation-harnesses/probe.json" <<'JSON'
{
  "name": "probe",
  "program": "bash",
  "argv": ["-c", "echo uid=$(id -u); echo interfaces=$(ls /sys/class/net | tr '\\n' ' '); for t in 1.1.1.1:443 8.8.8.8:53 169.254.169.254:80 172.17.0.1:22 172.17.0.1:8040 host.docker.internal:8040; do h=${t%:*}; p=${t#*:}; if timeout 3 bash -c \"exec 3<>/dev/tcp/$h/$p\" 2>/dev/null; then echo \"OPEN $t\"; else echo \"closed $t\"; fi; done; if getent hosts example.com >/dev/null 2>&1; then echo 'OPEN dns'; else echo 'closed dns'; fi; echo \"docker-socket=$(ls /var/run/docker.sock /run/docker.sock 2>&1 | head -1)\"; echo \"server-data=$(ls /var/lib/gusto 2>&1 | head -1)\"; grep -E 'NoNewPrivs|Seccomp:|CapEff|CapBnd' /proc/self/status | tr -s '\\t ' ' '; echo kernel=$(uname -r)"],
  "needs": "nothing",
  "stream": "plain"
}
JSON

GUSTO_HARNESSES="$smoke/isolation-harnesses" gusto new "$smoke/isolation-job" -repo "$smoke/isolation-repo" \
  -brief "probe the box" -harness probe -- true >/dev/null
if [ -n "${GUSTO_API_TOKENS:-}" ] && [ -z "${GUSTO_TOKEN:-}" ]; then
  echo "this server takes API tokens: set GUSTO_TOKEN to one that may send jobs" >&2
  exit 2
fi
id=$(GUSTO_SERVER=http://127.0.0.1:8040 gusto submit "$smoke/isolation-job" | head -n 1)
echo "submitted $id"

tries=0
until [ -f "$GUSTO_JOBS/$id/out/manifest.json" ]; do
  tries=$((tries + 1))
  if [ "$tries" -gt 120 ]; then
    echo "no manifest after 120 seconds; the job page says why: /jobs/$id" >&2
    exit 1
  fi
  sleep 1
done

seen="$GUSTO_JOBS/$id/out/transcript.log"
echo "what the job saw:"
cat "$seen"

fail=0
need() { grep -q "$1" "$seen" || { echo "FAIL: $2" >&2; fail=1; }; }
need '^interfaces=lo *$' "the box has a network interface besides loopback"
if grep -q '^OPEN ' "$seen"; then echo "FAIL: the box opened a connection: $(grep '^OPEN ' "$seen" | tr '\n' ' ')" >&2; fail=1; fi
need '^closed dns$' "the box resolved a name"
grep -q '^docker-socket=.*No such file' "$seen" || { echo "FAIL: the box sees a container runtime socket" >&2; fail=1; }
grep -q '^server-data=.*No such file' "$seen" || { echo "FAIL: the box sees the server's data" >&2; fail=1; }
need 'NoNewPrivs: 1' "the box can gain a privilege"
[ "$fail" = 0 ] || exit 1
echo "the box has no network but loopback, no runtime socket, none of the server's data, and cannot gain a privilege"
