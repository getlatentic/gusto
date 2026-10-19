# Runs on the instance: a gusto server whose jobs run in OpenShell sandboxes made
# from the runner image, and one job for opencode on the sample project. The
# model is the one in ~/e2e/model.env when there is one, and a model that only
# answers "pong" when there is not.
set -uo pipefail
E=$HOME/e2e
pkill -f "[e]2e/gusto serve" 2>/dev/null || true; pkill -f "[f]ake_model.py" 2>/dev/null || true
rm -rf "$E/rdata" "$E/rsample" "$E/rjob"; mkdir -p "$E/rdata" "$E/rsample"
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
for old in $(openshell sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^gusto-/ {print $1}'); do openshell sandbox delete "$old" >/dev/null 2>&1; done

# The sample project: a discount that is a hundred times too large, and tests that say so.
cp "$E"/sample/* "$E/rsample/"
(cd "$E/rsample" && git init -q . && git add -A && git -c user.email=a@b -c user.name=x commit -qm sample)

if [ -f "$E/model.env" ]; then
  set -a; . "$E/model.env"; set +a
  echo "model: $GUSTO_MODEL_NAME/$GUSTO_MODEL_ID at $GUSTO_MODEL_UPSTREAM"
else
  python3 "$E/fake_model.py" >/dev/null 2>&1 </dev/null &
  GUSTO_MODEL_UPSTREAM=http://127.0.0.1:9000; GUSTO_MODEL_KEY=not-a-real-key; GUSTO_MODEL_NAME=deepseek; GUSTO_MODEL_ID=deepseek-flash
  echo "model: a fake one that answers pong"
fi
mkdir -p "$E/keys" && "$E/gusto" keygen e2e -dir "$E/keys" >/dev/null 2>&1

cd "$E"
GUSTO_PASSWORD=e2e GUSTO_ADDR=127.0.0.1:8040 GUSTO_SANDBOX=openshell GUSTO_OPENSHELL_IMAGE="$E/runner-rootfs.tar.gz" GUSTO_OPENSHELL_GATEWAY=spike \
  GUSTO_MODEL_UPSTREAM="$GUSTO_MODEL_UPSTREAM" GUSTO_MODEL_KEY="$GUSTO_MODEL_KEY" GUSTO_MODEL_NAME="$GUSTO_MODEL_NAME" GUSTO_MODELS="$GUSTO_MODEL_NAME/$GUSTO_MODEL_ID" \
  GUSTO_LOCAL_REPOS="$E/rsample" GUSTO_JOB_CONCURRENCY=1 GUSTO_SIGNING_KEY="$E/keys/e2e.key" GUSTO_KEYRING="$E/keys" GUSTO_SIGNER=e2e \
  GUSTO_OPENSHELL_MEMORY=3Gi GUSTO_OPENSHELL_CPU=2 \
  GUSTO_JOBS="$E/rdata/jobs" GUSTO_SESSIONS="$E/rdata/sessions" GUSTO_SETUPS="$E/rdata/setups" GUSTO_COMPARISONS="$E/rdata/comparisons" GUSTO_CHANGES="$E/rdata/changes/changes.jsonl" \
  setsid nohup "$E/gusto" serve > "$E/rserve.log" 2>&1 </dev/null &
for _ in $(seq 1 120); do curl -fsS http://127.0.0.1:8040/healthz >/dev/null 2>&1 && break; sleep 1; done
echo "== engines the image offers"; grep -E "opencode|not offered" "$E/rserve.log" | head -4 | cut -c1-200

"$E/gusto" new "$E/rjob" -repo "$E/rsample" -brief "The discount is a hundred times too large. Fix invoice.js so the tests in this project pass, and change nothing else." \
  -agent opencode -model "$GUSTO_MODEL_NAME/$GUSTO_MODEL_ID" -- node --test >/dev/null
id=$(GUSTO_SERVER=http://127.0.0.1:8040 GUSTO_PASSWORD=e2e "$E/gusto" submit "$E/rjob" | head -n 1)
echo "submitted $id"
for _ in $(seq 1 360); do [ -f "$E/rdata/jobs/$id/out/manifest.json" ] && break; grep -q '"state":"failed"' "$E/rdata/jobs/$id/state.json" 2>/dev/null && break; sleep 1; done
sleep 2
J=$E/rdata/jobs/$id
echo "== state";        tr -d '\n ' < "$J/state.json" | cut -c1-260; echo
echo "== transcript";   head -c 1500 "$J/out/transcript.log"; echo
echo "== stderr";       head -c 800 "$J/out/stderr.log"; echo
echo "== check";         tr -d '\n ' < "$J/out/gates.json" 2>/dev/null; echo
echo "== diff";         head -30 "$J/out/diff.patch" 2>/dev/null
echo "== proxy record"; cut -c1-220 "$J/out/proxy.jsonl" 2>/dev/null | head -8
echo "== guard: network"; grep -E "NET:OPEN|HTTP:|FINDING|DENIED|REFUSE" "$J/out/sandbox-events.log" 2>/dev/null | cut -c1-250 | head -30
echo "== files";        ls "$J/out" 2>/dev/null | tr '\n' ' '; echo
echo "== verify";       GUSTO_KEYRING="$E/keys" "$E/gusto" verify "$J" 2>&1 | tail -2
echo "== serve log";    grep -E "ERROR" "$E/rserve.log" | cut -c1-400 | tail -5
echo "== sandboxes left"; openshell sandbox list 2>&1 | head -4

# The same task given to opencode and pi from the pages' own form.
jar=$(mktemp); curl -s -c "$jar" -o /dev/null -d password=e2e http://127.0.0.1:8040/login
model=$(curl -s -b "$jar" http://127.0.0.1:8040/compare | python3 -c 'import re,sys,html; m=re.search(r"<select name=\"model\">\s*<option[^>]*>(.*?)</option>",sys.stdin.read(),re.S); print(html.unescape(m.group(1).strip()) if m else "")')
echo "== comparison, model offered: [$model]"
where=$(curl -s -b "$jar" -o /dev/null -w '%{redirect_url}' --data-urlencode "brief=The discount is a hundred times too large. Fix invoice.js so the tests pass, and change nothing else." \
  --data-urlencode "model=$model" -d harness=opencode -d harness=pi http://127.0.0.1:8040/compare)
echo "$where"
for _ in $(seq 1 240); do page=$(curl -s -b "$jar" "$where"); echo "$page" | grep -q "Working" || break; sleep 3; done
echo "$page" | python3 -c '
import re,sys,html
t=sys.stdin.read()
v=re.search(r"class=\"verdict\">(.*?)</p>",t,re.S)
print("verdict:",html.unescape(v.group(1).strip()) if v else "(none)")
for row in re.findall(r"<tr>(.*?)</tr>",t,re.S)[1:]:
    print(" | ".join(re.sub(r"\s+"," ",html.unescape(re.sub(r"<[^>]+>"," ",c))).strip() for c in re.findall(r"<t[dh][^>]*>(.*?)</t[dh]>",row,re.S)))
'
for J in "$E"/rdata/jobs/*/; do
  [ -f "$J/out/manifest.json" ] || continue
  echo "-- $(basename "$J"): agent $(grep -o '"harness":"[a-z]*"' "$J/state.json") $(grep -o '"state":"[a-z]*"' "$J/state.json"); $(GUSTO_KEYRING="$E/keys" "$E/gusto" verify "$J" 2>&1 | head -1); model calls: $(grep -c . "$J/out/proxy.jsonl" 2>/dev/null); denied: $(grep -c "DENIED" "$J/out/sandbox-events.log")"
done
echo "== serve log errors"; grep -E "ERROR" "$E/rserve.log" | cut -c1-300 | tail -5
echo "== sandboxes left"; openshell sandbox list 2>&1 | head -3
