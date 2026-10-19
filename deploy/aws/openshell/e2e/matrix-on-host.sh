# Runs on the instance: gusto benchmark, from the plan in ~/e2e/matrix/plan.json, with every
# cell started in an OpenShell sandbox made from the image that carries all the harnesses.
# The credentials are built here from Parameter Store with the instance's role, so no
# key is anywhere else. It runs detached and writes ~/e2e/matrix/done when it ends.
set -uo pipefail
E=$HOME/e2e; M=$E/matrix
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
for old in $(openshell sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^gusto-/ {print $1}'); do openshell sandbox delete "$old" >/dev/null 2>&1; done

mkdir -p "$M/keys" "$M/creds" "$M/sample"
# With AGAIN set the matrix that is there is kept, and only the pairs it names run again.
[ -n "${AGAIN:-}" ] || rm -rf "$M/plan-matrix" "$M/one-line-fix"
rm -rf "$M/done" "$M/run.log"
# The job is pinned to this repository's commit, so a run that keeps the matrix keeps it too.
if [ ! -d "$M/sample/.git" ]; then
  cp "$E"/sample/* "$M/sample/"
  (cd "$M/sample" && git init -q . && git add -A && git -c user.email=a@b -c user.name=x commit -qm sample)
fi
[ -f "$M/keys/e2e.key" ] || "$E/gusto" keygen e2e -dir "$M/keys" >/dev/null 2>&1

bash "$E/credentials.sh" "$M/creds"
[ -s "$M/creds/deepseek.json" ] || { echo "no credentials: is /gusto/poc/deepseek-key stored? (./put-key.sh)"; touch "$M/done"; exit 1; }

# The one job every pair is given, with a time limit a hung harness cannot outlast.
[ -d "$M/one-line-fix" ] || "$E/gusto" new "$M/one-line-fix" -repo "$M/sample" -brief "The discount is a hundred times too large. Fix invoice.js so the tests in this project pass, and change nothing else." \
  -agent opencode -model platform/qwen.qwen3-vl-235b-a22b-instruct -- node --test >/dev/null
[ -n "${AGAIN:-}" ] || python3 - "$M/one-line-fix/job.json" <<'PY'
import json, sys
spec = json.load(open(sys.argv[1]))
spec["budget"]["seconds"] = 420
json.dump(spec, open(sys.argv[1], "w"), indent=2)
PY

cd "$M"
GUSTO_SANDBOX=openshell GUSTO_OPENSHELL_IMAGE="$E/runner-all-rootfs.tar.gz" GUSTO_OPENSHELL_GATEWAY=spike \
  GUSTO_OPENSHELL_MEMORY=3Gi GUSTO_OPENSHELL_CPU=2 GUSTO_CREDENTIALS="$M/creds" \
  GUSTO_SIGNING_KEY="$M/keys/e2e.key" GUSTO_KEYRING="$M/keys" GUSTO_SIGNER=e2e \
  "$E/gusto" benchmark ${AGAIN:+-again "$AGAIN"} "$M/plan.json" > "$M/table.txt" 2> "$M/run.log"
echo "exit $?" >> "$M/run.log"
touch "$M/done"
