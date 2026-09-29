# Runs on the instance: measures the host, then starts sandboxes and runs
# inside.sh in one.
set -uo pipefail
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
echo "host kernel: $(uname -r); /dev/kvm: $(ls /dev/kvm 2>&1)"
echo "openshell: $(openshell --version)"
openshell gateway select spike >/dev/null 2>&1
encoded=$(base64 -w0 /opt/openshell/inside.sh)
for run in cold warm; do
  start=$(date +%s.%N)
  out=$(openshell sandbox create --name "probe-$run" -- sh -c "echo $encoded | base64 -d | bash" 2>&1 | grep -vE '^\s*\[[0-9.]+s\]|^$')
  end=$(date +%s.%N)
  printf 'sandbox start and command, %s: %.1f s\n' "$run" "$(echo "$end - $start" | bc)"
  [ "$run" = cold ] || echo "$out"
  openshell sandbox delete "probe-$run" >/dev/null 2>&1
done
