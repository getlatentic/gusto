# Runs on the instance: a gusto server whose jobs run in OpenShell sandboxes, a
# model that always says pong, and one job sent through the API.
set -uo pipefail
E=$HOME/e2e
pkill -f "[e]2e/gusto serve" 2>/dev/null || true; pkill -f "[f]ake_model.py" 2>/dev/null || true
rm -rf "$E/data" "$E/repo" "$E/job"; mkdir -p "$E/data" "$E/repo" "$E/harnesses"
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls

for old in $(openshell sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^gusto-/ {print $1}'); do openshell sandbox delete "$old" >/dev/null 2>&1; done

# A repository whose gate fails until answer.txt says yes.
cd "$E/repo" && git init -q . && printf no > answer.txt && printf '#!/bin/sh\ntest "$(cat answer.txt)" = yes\n' > test.sh && git add -A \
  && git -c user.email=a@b -c user.name=x commit -qm start
python3 "$E/fake_model.py" >/dev/null 2>&1 </dev/null &
scripts=$(base64 -w0 "$E/inside-harness.sh")
cat > "$E/harnesses/fixture.json" <<JSON
{"name":"fixture","program":"bash","argv":["-c","echo $scripts | base64 -d | bash"],"needs":"nothing","stream":"plain"}
JSON

cd "$E"
GUSTO_PASSWORD=e2e GUSTO_ADDR=127.0.0.1:8040 GUSTO_SANDBOX=openshell GUSTO_MODEL_UPSTREAM=http://127.0.0.1:9000 GUSTO_MODEL_KEY=not-a-real-key \
  GUSTO_LOCAL_REPOS="$E/repo" GUSTO_HARNESSES="$E/harnesses" \
  GUSTO_JOBS="$E/data/jobs" GUSTO_SESSIONS="$E/data/sessions" GUSTO_SETUPS="$E/data/setups" GUSTO_COMPARISONS="$E/data/comparisons" GUSTO_CHANGES="$E/data/changes/changes.jsonl" \
  GUSTO_ALLOWED_HOSTS="registry.npmjs.org" GUSTO_OPENSHELL_GATEWAY=spike GUSTO_JOB_CONCURRENCY=1 \
  setsid nohup "$E/gusto" serve > "$E/serve.log" 2>&1 </dev/null &
for _ in $(seq 1 30); do curl -fsS http://127.0.0.1:8040/healthz >/dev/null 2>&1 && break; sleep 1; done

GUSTO_HARNESSES="$E/harnesses" "$E/gusto" new "$E/job" -repo "$E/repo" -brief "make the answer yes" -agent fixture -- sh test.sh >/dev/null
id=$(GUSTO_SERVER=http://127.0.0.1:8040 GUSTO_PASSWORD=e2e "$E/gusto" submit "$E/job" | head -n 1)
echo "submitted $id"
for _ in $(seq 1 120); do [ -f "$E/data/jobs/$id/out/manifest.json" ] && break; sleep 1; done
sleep 1
J=$E/data/jobs/$id
echo "== state";        cat "$J/state.json" | tr -d '\n '; echo
echo "== transcript";   cat "$J/out/transcript.log"
echo "== check";         cat "$J/out/gates.json" | tr -d '\n '; echo
echo "== diff";         cat "$J/out/diff.patch"
echo "== proxy record"; cat "$J/out/proxy.jsonl" | cut -c1-260
echo "== sandbox events"; cat "$J/out/sandbox-events.log" | cut -c1-260
echo "== sandbox policy"; tr -d '\n ' < "$J/out/sandbox-policy.json" | cut -c1-400; echo
echo "== files";        ls "$J/out"
echo "== verify";       "$E/gusto" verify "$J" | tail -3 || true
echo "== serve log";    tail -n 12 "$E/serve.log" | cut -c1-260
echo "== sandboxes left"; openshell sandbox list 2>&1 | head -5
