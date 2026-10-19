# Runs on the instance: N sandboxes made at once on one gateway, at the size gusto uses (2 CPU,
# 3 GiB). For each N it reports how long every sandbox took to be ready, how long the same
# fixed piece of CPU work took inside each of them at the same time, and the host's memory
# and load. This is the measurement getlatentic/planning#426 asks for.
set -uo pipefail
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
G="openshell --gateway spike"
IMAGE=$HOME/e2e/runner-all-rootfs.tar.gz
POLICY=$(mktemp)
echo '{"version":1,"filesystem_policy":{"include_workdir":false,"read_only":["/usr","/lib","/bin","/etc","/opt","/proc","/dev/urandom","/var/log"],"read_write":["/sandbox","/tmp","/dev/null","/dev/ptmx","/dev/pts"]},"network_policies":{}}' > "$POLICY"
for old in $($G sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^(gusto|conc)-/ {print $1}'); do $G sandbox delete "$old" >/dev/null 2>&1; done
pkill -f "[o]penshell.*forward" 2>/dev/null || true
WORK="let t=Date.now(),x=0;for(let i=0;i<4e8;i++)x+=i;console.log(Date.now()-t)"

ready() {  # makes sandbox $1 and prints how many seconds it took to answer a command, or why it did not
  local start=$(date +%s.%N)
  if ! $G sandbox create --name "$1" --policy "$POLICY" --cpu 2 --memory 3Gi --no-tty --detach --no-auto-providers --from "$IMAGE" -- sleep infinity >/dev/null 2>"/tmp/$1.err" </dev/null; then
    echo "FAILED: $(tr '\n' ' ' < "/tmp/$1.err" | cut -c1-100)"; return
  fi
  local tries=0
  until $G sandbox exec --name "$1" --no-login-shell --no-tty -- true >/dev/null 2>&1 </dev/null; do
    sleep 0.5; tries=$((tries + 1)); [ "$tries" -lt 240 ] || { echo "TIMED OUT"; return; }
  done
  echo "$(date +%s.%N) $start" | awk '{printf "%.1f", $1-$2}'
}

printf '%-3s  %-28s  %-34s  %s\n' N "ready in (s), each" "the same CPU work in (ms), each" "host after"
WAVE=${WAVE:-4}   # the gateway allows about four rootfs archives to be staged at once
for n in ${SIZES:-1 2 4 8 12}; do
  rm -f /tmp/conc-*.t /tmp/conc-*.w
  for first in $(seq 1 "$WAVE" "$n"); do
    for i in $(seq "$first" $((first + WAVE - 1))); do [ "$i" -le "$n" ] && ( ready "conc-$i" > "/tmp/conc-$i.t" ) & done; wait
  done
  for i in $(seq 1 "$n"); do grep -q "^[0-9.]*$" "/tmp/conc-$i.t" && ( $G sandbox exec --name "conc-$i" --no-login-shell --no-tty -- node -e "$WORK" > "/tmp/conc-$i.w" 2>&1 </dev/null ) & done; wait
  printf '%-3s  %-28s  %-34s  %s\n' "$n" "$(cat /tmp/conc-*.t | sort -n | tr '\n' ' ')" "$(cat /tmp/conc-*.w | sort -n | tr '\n' ' ')" \
    "$(free -m | awk 'NR==2{printf "used %d MB, free %d MB", $3, $4}'), load $(cut -d' ' -f1 /proc/loadavg)"
  for i in $(seq 1 "$n"); do $G sandbox delete "conc-$i" >/dev/null 2>&1 & done; wait
done
rm -f "$POLICY" /tmp/conc-*.t /tmp/conc-*.w
$G sandbox list 2>&1 | head -3
