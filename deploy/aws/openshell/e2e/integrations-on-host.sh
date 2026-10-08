# Runs on the instance: a haas server whose jobs run in OpenShell sandboxes, an integration imported through its console, turned on and
# given to a setup, and Claude Code and Codex each asked, from the pages' own form, for a customer's account id, which only the
# integration's tool and skill can answer. The customer system is a server on the instance that answers only the key the gateway
# holds. It prints PASS or FAIL for each thing it checks, and the evidence under each.
set -uo pipefail
E=$HOME/e2e; R=$E/integ
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
pkill -f "[e]2e/haas serve" 2>/dev/null || true; pkill -f "[c]ustomers_server.py" 2>/dev/null || true; sleep 1
for old in $(openshell sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^haas-/ {print $1}'); do openshell sandbox delete "$old" >/dev/null 2>&1; done
rm -rf "$R"; mkdir -p "$R/keys" "$R/creds" "$R/sample"
cp "$E"/sample/* "$R/sample/"; (cd "$R/sample" && git init -q . && git add -A && git -c user.email=a@b -c user.name=x commit -qm sample)
"$E/haas" keygen e2e -dir "$R/keys" >/dev/null 2>&1
bash "$E/credentials.sh" "$R/creds"
[ -s "$R/creds/deepseek.json" ] || { echo "FAIL no credentials: is /haas/poc/deepseek-key stored? (./put-key.sh)"; exit 1; }

python3 "$E/customers_server.py" "$R/customers.log" >/dev/null 2>&1 </dev/null &
sleep 1

# The plugin: a skill with a reference file beside it, and the customer system as its server. The key is in the package, which the
# format allows as fixed headers and which is no secret mechanism; what is checked is that the sandbox never gets to use it.
python3 - "$R/plugin.zip" <<'PY'
import json, sys, zipfile
z = zipfile.ZipFile(sys.argv[1], "w")
z.writestr("plugin.json", json.dumps({"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json", "name": "customers", "version": "1.0.0", "description": "Look customers up."}))
z.writestr("skills/customer-account/SKILL.md", "---\nname: customer-account\ndescription: Use when asked for a customer's account id, or anything about a customer's account. Says how to answer.\n---\n\nTo find a customer's account id, use the customer tools you were given: search for the tool, read how to call it, and call it with the customer's name. Then read references/answer-format.md in this skill's folder and answer in exactly that format.\n")
z.writestr("skills/customer-account/references/answer-format.md", "Answer in one line: ACCOUNT-ANSWER: <the account id, exactly as the tool gave it>\n")
z.writestr("mcp.json", json.dumps({"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "mcpServers": {"customers": {"type": "streamable-http", "url": "http://127.0.0.1:9100/mcp", "headers": {"X-Api-Key": "e2e-customer-system-key"}}}}))
z.close()
PY

cd "$R"
HAAS_PASSWORD=e2e HAAS_ADDR=127.0.0.1:8040 HAAS_SANDBOX=openshell HAAS_OPENSHELL_IMAGE="$E/runner-all-rootfs.tar.gz" HAAS_OPENSHELL_GATEWAY=spike \
  HAAS_OPENSHELL_MEMORY=3Gi HAAS_OPENSHELL_CPU=2 HAAS_CREDENTIALS="$R/creds" HAAS_MODELS="anthropic/deepseek-flash,deepseek/deepseek-flash" \
  HAAS_GATEWAY_PRIVATE=1 HAAS_INTEGRATIONS_PRIVATE=1 HAAS_ADMINS="" HAAS_LOCAL_REPOS="$R/sample" HAAS_JOB_CONCURRENCY=1 \
  HAAS_SIGNING_KEY="$R/keys/e2e.key" HAAS_KEYRING="$R/keys" HAAS_SIGNER=e2e \
  HAAS_JOBS="$R/data/jobs" HAAS_SESSIONS="$R/data/sessions" HAAS_SETUPS="$R/data/setups" HAAS_INTEGRATIONS="$R/data/integrations" \
  HAAS_COMPARISONS="$R/data/comparisons" HAAS_CHANGES="$R/data/changes/changes.jsonl" \
  setsid nohup "$E/haas" serve > "$R/serve.log" 2>&1 </dev/null &
for _ in $(seq 1 120); do curl -fsS http://127.0.0.1:8040/healthz >/dev/null 2>&1 && break; sleep 1; done
H=http://127.0.0.1:8040; jar=$R/jar
sleep 3
for _ in 1 2 3 4 5; do [ "$(curl -s -c "$jar" -o /dev/null -w "%{http_code}" -d password=e2e "$H/login")" = 303 ] && break; sleep 2; done
loc() { curl -s -b "$jar" -o /dev/null -w '%{redirect_url}' "$@"; }
ok() { if [ "$1" = 0 ]; then echo "PASS $2"; else echo "FAIL $2"; fi; }

echo "== the console: import, off, turn on, a skill set that names it"
where=$(loc -F kind=upload -F "archive=@$R/plugin.zip;filename=customers.zip" "$H/admin/integrations/import")
echo "imported: $where"
version=${where##*version=}
page=$(curl -s -b "$jar" "$H/admin/integrations/customers?version=$version")
echo "$page" | grep -q "customer-account" && echo "$page" | grep -q "customers" ; ok $? "the page shows the skill and the server of the plugin"
[ "$(ls "$R/data/integrations/customers" 2>/dev/null | head -1)" = "integration.json" ]; ok $? "an imported integration is kept"
python3 - "$R/data/integrations/customers/integration.json" <<'PY' ; ok $? "it is off until an administrator turns it on"
import json,sys; sys.exit(0 if json.load(open(sys.argv[1])).get("on","")=="" else 1)
PY
loc -d name=customers -d integration=customers "$H/admin/skill-sets" >/dev/null
loc "$H/admin/skill-sets/customers/enable" -X POST >/dev/null
# While the integration is off, an agent started with the set is refused when it starts a turn.
off=$(loc -d harness=claude -d model=anthropic/deepseek-flash -d setup=customers "$H/instances")
echo "an instance started while it is off: $off"
loc -d version="$version" "$H/admin/integrations/customers/enable" >/dev/null
python3 - "$R/data/integrations/customers/integration.json" "$version" <<'PY' ; ok $? "turning the version on is what makes it on"
import json,sys; sys.exit(0 if json.load(open(sys.argv[1])).get("on")==sys.argv[2] else 1)
PY

run() {
  harness=$1; model=$2
  echo; echo "=================== $harness on $model"
  inst=$(loc -d "harness=$harness" -d "model=$model" -d setup=customers "$H/instances")
  echo "instance: $inst"
  said=$(loc --data-urlencode "said=What is the account id of the customer called Acme? Use your customer tools, not a guess." "${inst}/say")
  id=${said##*#}; J=$R/data/jobs/$id
  echo "job: $id"
  for _ in $(seq 1 420); do [ -f "$J/out/manifest.json" ] && break; grep -q '"state":"failed"' "$J/state.json" 2>/dev/null && break; sleep 2; done
  sleep 2
  echo "-- state"; tr -d '\n ' < "$J/state.json" 2>/dev/null | cut -c1-240; echo
  echo "-- transcript (tail)"; tail -c 1200 "$J/out/transcript.log" 2>/dev/null; echo
  echo "-- stderr"; head -c 600 "$J/out/stderr.log" 2>/dev/null; echo
  echo "-- gateway record"; cut -c1-260 "$J/out/gateway.jsonl" 2>/dev/null
  echo "-- setup in the record"; python3 -c "import json;print(json.dumps(json.load(open('$J/job.json')).get('setup')))" 2>/dev/null
  echo "-- sandbox events: denied"; grep -E "DENIED|NET:OPEN|FINDING" "$J/out/sandbox-events.log" 2>/dev/null | cut -c1-220 | head -8
  grep -q 'CUST-7391-ZETA' "$J/out/transcript.log" 2>/dev/null; ok $? "$harness: the answer has the id only the customer system holds"
  grep -q 'ACCOUNT-ANSWER:' "$J/out/transcript.log" 2>/dev/null; ok $? "$harness: it used the skill's reference file for the format"
  grep -q '"kind":"call"' "$J/out/gateway.jsonl" 2>/dev/null && grep -q '"outcome":"ok"' "$J/out/gateway.jsonl" 2>/dev/null; ok $? "$harness: the call is in the job's gateway record"
  grep -q '"person"' "$J/out/gateway.jsonl" 2>/dev/null; ok $? "$harness: the record says who asked"
  grep -rq 'e2e-customer-system-key' "$J" 2>/dev/null && ! grep -rlq 'e2e-customer-system-key' "$J/out" 2>/dev/null; ok $? "$harness: the customer system's key is in the job's private setup folder and not in anything it wrote"
  ! grep -rq 'e2e-customer-system-key' "$J/out" 2>/dev/null; ok $? "$harness: the key is nowhere in the job's evidence"
  HAAS_KEYRING="$R/keys" "$E/haas" verify "$J" >/dev/null 2>&1; ok $? "$harness: the signed record verifies"
}
run claude anthropic/deepseek-flash
run codex deepseek/deepseek-flash

echo; echo "== the customer system's own log: every request was keyed, and came from the gateway on this machine"
sort "$R/customers.log" | uniq -c | head
! grep -q '"keyed": false' "$R/customers.log"; ok $? "no request reached the customer system without the key"
echo "== change record"; cut -c1-200 "$R/data/changes/changes.jsonl" | grep -E "integration|setup" | head -8
echo "== serve log errors"; grep -E "ERROR" "$R/serve.log" | cut -c1-300 | tail -5
echo "== sandboxes left"; openshell sandbox list 2>&1 | head -4
touch "$R/done"
