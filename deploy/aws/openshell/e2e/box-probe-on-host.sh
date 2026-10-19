# Runs on the instance: a gusto server whose jobs run in OpenShell sandboxes, and one job whose harness is the probe of
# deploy/smoke-isolation.sh, so a container and a sandbox are asked the same questions. It prints what the job saw and the
# host's own kernel, since a sandbox is a microVM and should not share it.
set -uo pipefail
E=$HOME/e2e
pkill -f "[e]2e/gusto serve" 2>/dev/null || true; pkill -f "[f]ake_model.py" 2>/dev/null || true
rm -rf "$E/data" "$E/repo" "$E/job"; mkdir -p "$E/data" "$E/repo" "$E/harnesses"
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
for old in $(openshell sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^gusto-/ {print $1}'); do openshell sandbox delete "$old" >/dev/null 2>&1; done

cd "$E/repo" && git init -q . && echo "nothing to change" > README && git add -A && git -c user.email=a@b -c user.name=x commit -qm start
python3 "$E/fake_model.py" >/dev/null 2>&1 </dev/null &
cp "$E/box-probe.json" "$E/harnesses/probe.json"

cd "$E"
GUSTO_PASSWORD=e2e GUSTO_ADDR=127.0.0.1:8040 GUSTO_SANDBOX=openshell GUSTO_MODEL_UPSTREAM=http://127.0.0.1:9000 GUSTO_MODEL_KEY=not-a-real-key \
  GUSTO_LOCAL_REPOS="$E/repo" GUSTO_HARNESSES="$E/harnesses" \
  GUSTO_JOBS="$E/data/jobs" GUSTO_SESSIONS="$E/data/sessions" GUSTO_SETUPS="$E/data/setups" GUSTO_COMPARISONS="$E/data/comparisons" GUSTO_CHANGES="$E/data/changes/changes.jsonl" \
  GUSTO_OPENSHELL_GATEWAY=spike GUSTO_JOB_CONCURRENCY=1 \
  setsid nohup "$E/gusto" serve > "$E/serve.log" 2>&1 </dev/null &
for _ in $(seq 1 30); do curl -fsS http://127.0.0.1:8040/healthz >/dev/null 2>&1 && break; sleep 1; done

GUSTO_HARNESSES="$E/harnesses" "$E/gusto" new "$E/job" -repo "$E/repo" -brief "probe the box" -agent probe -- true >/dev/null
id=$(GUSTO_SERVER=http://127.0.0.1:8040 GUSTO_PASSWORD=e2e "$E/gusto" submit "$E/job" | head -n 1)
echo "submitted $id"
for _ in $(seq 1 180); do [ -f "$E/data/jobs/$id/out/manifest.json" ] && break; sleep 1; done
sleep 1
J=$E/data/jobs/$id
echo "== what the task saw"; cat "$J/out/transcript.log"
echo "== the host's kernel"; uname -r
echo "== state"; tr -d '\n ' < "$J/state.json"; echo
echo "== serve log"; tail -n 6 "$E/serve.log" | cut -c1-240
