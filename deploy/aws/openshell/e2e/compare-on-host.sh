# Runs on the instance: a haas server whose jobs run in OpenShell sandboxes made
# from a Node root file system, two engines (one fixes the sample's bug and one
# does nothing), and one comparison started from the pages' own form.
set -uo pipefail
E=$HOME/e2e
pkill -f "[e]2e/haas serve" 2>/dev/null || true
rm -rf "$E/cdata" "$E/charnesses"; mkdir -p "$E/cdata" "$E/charnesses"
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
for old in $(openshell sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^haas-/ {print $1}'); do openshell sandbox delete "$old" >/dev/null 2>&1; done

cat > "$E/charnesses/fixer.json" <<'JSON'
{"name":"fixer","program":"bash","skills":"/root/.agents/skills","needs":"nothing","stream":"plain",
 "argv":["-c","sed -i 's#subtotal \\* discountPercent;#subtotal * discountPercent / 100;#' invoice.js && echo fixed the discount"]}
JSON
cat > "$E/charnesses/idler.json" <<'JSON'
{"name":"idler","program":"bash","skills":"/root/.agents/skills","needs":"nothing","stream":"plain",
 "argv":["-c","echo I looked at it and changed nothing"]}
JSON

mkdir -p "$E/keys" && "$E/haas" keygen e2e -dir "$E/keys" >/dev/null 2>&1
cd "$E"
HAAS_PASSWORD=e2e HAAS_ADDR=127.0.0.1:8040 HAAS_SANDBOX=openshell HAAS_OPENSHELL_IMAGE="$E/node-rootfs.tar.gz" HAAS_OPENSHELL_GATEWAY=spike \
  HAAS_MODEL_UPSTREAM=http://127.0.0.1:9000 HAAS_MODEL_KEY=not-a-real-key HAAS_MODEL_NAME=platform HAAS_MODELS=platform/fake \
  HAAS_HARNESSES="$E/charnesses" HAAS_JOB_CONCURRENCY=2 HAAS_SIGNING_KEY="$E/keys/e2e.key" HAAS_KEYRING="$E/keys" HAAS_SIGNER=e2e \
  HAAS_JOBS="$E/cdata/jobs" HAAS_SESSIONS="$E/cdata/sessions" HAAS_SETUPS="$E/cdata/setups" HAAS_COMPARISONS="$E/cdata/comparisons" HAAS_CHANGES="$E/cdata/changes/changes.jsonl" \
  setsid nohup "$E/haas" serve > "$E/cserve.log" 2>&1 </dev/null &
for _ in $(seq 1 90); do curl -fsS http://127.0.0.1:8040/healthz >/dev/null 2>&1 && break; sleep 1; done
echo "== engines the sandbox image offers"; grep -E "fixer|idler" "$E/cserve.log" | head -4

jar=$(mktemp)
curl -s -c "$jar" -o /dev/null -d password=e2e http://127.0.0.1:8040/login
# The model is chosen as the form offers it, by the name the page gives it.
model=$(curl -s -b "$jar" http://127.0.0.1:8040/compare | python3 -c 'import re,sys,html; m=re.search(r"<select name=\"model\">\s*<option[^>]*>(.*?)</option>",sys.stdin.read(),re.S); print(html.unescape(m.group(1).strip()) if m else "")')
echo "model offered: [$model]"
where=$(curl -s -b "$jar" -o /dev/null -w '%{redirect_url}' --data-urlencode "brief=Fix the discount so the tests pass" \
  --data-urlencode "model=$model" -d harness=fixer -d harness=idler http://127.0.0.1:8040/compare)
echo "== comparison: $where"
for _ in $(seq 1 180); do
  page=$(curl -s -b "$jar" "$where")
  echo "$page" | grep -q "Working" || break
  sleep 2
done
echo "$page" | python3 -c '
import re,sys,html
t=sys.stdin.read()
v=re.search(r"class=\"verdict\">(.*?)</p>",t,re.S)
print("verdict:",html.unescape(v.group(1).strip()) if v else "(none)")
for row in re.findall(r"<tr>(.*?)</tr>",t,re.S)[1:]:
    print(" | ".join(re.sub(r"\s+"," ",html.unescape(re.sub(r"<[^>]+>"," ",c))).strip() for c in re.findall(r"<t[dh][^>]*>(.*?)</t[dh]>",row,re.S)))
'
echo "== each run's record"
for J in "$E"/cdata/jobs/*/; do
  id=$(basename "$J"); [ -f "$J/out/manifest.json" ] || continue
  echo "-- $id: $(tr -d '\n ' < "$J/state.json" | cut -c1-120)"
  ls "$J/out" | tr '\n' ' '; echo
  HAAS_KEYRING="$E/keys" "$E/haas" verify "$J" 2>&1 | tail -2
  grep -cE "NET:|HTTP:" "$J/out/sandbox-events.log" | sed 's/^/network events in the guard log: /'
done
echo "== sandboxes left"; openshell sandbox list 2>&1 | head -4
echo "== serve log errors"; grep -E "ERROR" "$E/cserve.log" | cut -c1-300 | tail -5
