#!/bin/sh
# A job that is stopped while it runs, in a real container, and what that leaves. It is run twice inside the server, around the
# stop, which the check makes from outside with a signed-in session:
#
#   docker compose exec -T server sh -s -- start < ../smoke-stop.sh         # makes a job that will not end by itself
#   curl ... -X POST http://127.0.0.1:8040/jobs/<id>/stop                    # a person stops it
#   docker compose exec -T server sh -s -- check <id> < ../smoke-stop.sh    # what the stop left
#
# The harness is a fixture that writes a file and then sleeps for ten minutes, so the only way the job ends in the time this
# waits is that it was stopped.
set -eu
mode=${1:?start or check}

# state is the word a job's record has for how it stands. The file is indented, so spaces and newlines are taken out first.
state() { tr -d ' \n' < "$GUSTO_JOBS/$1/state.json" 2>/dev/null | sed -n 's/.*"state":"\([a-z]*\)".*/\1/p'; }
data=$(dirname "$GUSTO_JOBS")
smoke="$data/smoke"

case "$mode" in
start)
  rm -rf "$smoke/stop-job"
  mkdir -p "$smoke/stop-repo" "$smoke/stop-harnesses"
  cd "$smoke/stop-repo"
  if [ ! -d .git ]; then
    echo "nothing to change" > README
    git init -q . && git add -A
    git -c user.email=gusto@localhost -c user.name=gusto commit -qm "a repository to stop in"
  fi
  cat > "$smoke/stop-harnesses/sleeper.json" <<'JSON'
{
  "name": "sleeper",
  "program": "sh",
  "argv": ["-c", "echo did-this-before-it-was-stopped > made.txt; sleep 600 & wait"],
  "needs": "nothing",
  "stream": "plain"
}
JSON
  GUSTO_HARNESSES="$smoke/stop-harnesses" gusto new "$smoke/stop-job" -repo "$smoke/stop-repo" \
    -brief "work until stopped" -harness sleeper -- true >/dev/null
  if [ -n "${GUSTO_API_TOKENS:-}" ] && [ -z "${GUSTO_TOKEN:-}" ]; then
    echo "this server takes API tokens: set GUSTO_TOKEN to one that may send jobs" >&2
    exit 2
  fi
  id=$(GUSTO_SERVER=http://127.0.0.1:8040 gusto submit "$smoke/stop-job" | head -n 1)
  tries=0
  until [ "$(state "$id")" = running ] && [ -f "$GUSTO_JOBS/$id/workspace/made.txt" ]; do
    tries=$((tries + 1))
    [ "$tries" -le 120 ] || { echo "the job was not running with its work done after 120 seconds" >&2; exit 1; }
    sleep 1
  done
  echo "$id"
  ;;
check)
  id=${2:?the id of the job}
  tries=0
  until [ "$(state "$id")" = interrupted ]; do
    tries=$((tries + 1))
    [ "$tries" -le 90 ] || { echo "FAIL: the job did not end after it was stopped:" >&2; cat "$GUSTO_JOBS/$id/state.json" >&2; exit 1; }
    sleep 1
  done
  fail=0
  tr -d ' \n' < "$GUSTO_JOBS/$id/state.json" | grep -q '"stoppedBy"' || { echo "FAIL: the job's record does not say who stopped it" >&2; fail=1; }
  grep -q '"stoppedBy"' "$GUSTO_JOBS/$id/out/manifest.json" || { echo "FAIL: the manifest does not say who stopped it" >&2; fail=1; }
  grep -q did-this-before-it-was-stopped "$GUSTO_JOBS/$id/workspace/made.txt" || { echo "FAIL: what the job had done was not kept" >&2; fail=1; }
  left=$(docker ps -a --filter "name=gusto-$id" -q)
  [ -z "$left" ] || { echo "FAIL: the job's container is still there: $left" >&2; fail=1; }
  GUSTO_KEYRING="${GUSTO_KEYRING:-}" gusto verify "$GUSTO_JOBS/$id" >/dev/null 2>&1 || echo "note: verify did not hold (a stopped job has no gate to re-run)"
  [ "$fail" = 0 ] || exit 1
  grep -o '"stoppedBy":"[^"]*"' "$GUSTO_JOBS/$id/out/manifest.json" | head -1
  echo "the job was stopped, ended interrupted, kept its work, and its container is gone"
  ;;
*) echo "start or check" >&2; exit 2 ;;
esac
