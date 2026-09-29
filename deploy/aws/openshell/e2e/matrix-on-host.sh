# Runs on the instance: haas matrix, from the plan in ~/e2e/matrix/plan.json, with every
# cell started in an OpenShell sandbox made from the image that carries all the harnesses.
# The credentials are built here from Parameter Store with the instance's role, so no
# key is anywhere else. It runs detached and writes ~/e2e/matrix/done when it ends.
set -uo pipefail
E=$HOME/e2e; M=$E/matrix
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
for old in $(openshell sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^haas-/ {print $1}'); do openshell sandbox delete "$old" >/dev/null 2>&1; done

mkdir -p "$M/keys" "$M/creds" "$M/sample"
rm -rf "$M/plan-matrix" "$M/one-line-fix" "$M/done" "$M/run.log"
cp "$E"/sample/* "$M/sample/"
(cd "$M/sample" && rm -rf .git && git init -q . && git add -A && git -c user.email=a@b -c user.name=x commit -qm sample)
[ -f "$M/keys/e2e.key" ] || "$E/haas" keygen e2e -dir "$M/keys" >/dev/null 2>&1

# Which credentials there are, and which harnesses each answers, as greencloud's host_vars say.
python3 - "$BEDROCK_PARAMETER" "$DEEPSEEK_PARAMETER" "$M/creds" <<'PY'
import json, os, subprocess, sys
def secret(name):
    return subprocess.run(["aws", "ssm", "get-parameter", "--with-decryption", "--name", name,
                           "--query", "Parameter.Value", "--output", "text"],
                          check=True, capture_output=True, text=True).stdout.strip()
bedrock, deepseek = secret(sys.argv[1]), secret(sys.argv[2])
held = {
    "platform": {"address": "https://bedrock-mantle.us-east-1.api.aws", "auth": "bearer", "key": bedrock, "harnesses": ["opencode"]},
    "deepseek": {"address": "https://api.deepseek.com", "auth": "bearer", "key": deepseek},
    "anthropic": {"address": "https://api.deepseek.com/anthropic", "auth": "anthropic", "key": deepseek, "harnesses": ["claude"]},
}
for name, credential in held.items():
    fd = os.open(os.path.join(sys.argv[3], name + ".json"), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as out:
        json.dump(credential, out)
PY
[ -s "$M/creds/deepseek.json" ] || { echo "no credentials: is /haas/poc/deepseek-key stored? (./put-key.sh)"; touch "$M/done"; exit 1; }

# The one job every pair is given, with a time limit a hung harness cannot outlast.
"$E/haas" new "$M/one-line-fix" -repo "$M/sample" -brief "The discount is a hundred times too large. Fix invoice.js so the tests in this project pass, and change nothing else." \
  -harness opencode -model platform/qwen.qwen3-vl-235b-a22b-instruct -- node --test >/dev/null
python3 - "$M/one-line-fix/job.json" <<'PY'
import json, sys
spec = json.load(open(sys.argv[1]))
spec["budget"]["seconds"] = 420
json.dump(spec, open(sys.argv[1], "w"), indent=2)
PY

cd "$M"
HAAS_SANDBOX=openshell HAAS_OPENSHELL_IMAGE="$E/runner-all-rootfs.tar.gz" HAAS_OPENSHELL_GATEWAY=spike \
  HAAS_OPENSHELL_MEMORY=3Gi HAAS_OPENSHELL_CPU=2 HAAS_CREDENTIALS="$M/creds" \
  HAAS_SIGNING_KEY="$M/keys/e2e.key" HAAS_KEYRING="$M/keys" HAAS_SIGNER=e2e \
  "$E/haas" matrix "$M/plan.json" > "$M/table.txt" 2> "$M/run.log"
echo "exit $?" >> "$M/run.log"
touch "$M/done"
