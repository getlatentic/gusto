#!/bin/sh
# Upgrade and roll back on data a published release made.
#
# For each release named, it builds that release, has it make a signed job and
# the directories a server keeps, upgrades the data with this checkout, and then
# rolls it back and starts the old release on it again. Each step reads the job
# back: the page renders and `haas verify` holds.
#
#   deploy/upgrade-check.sh v0.1.0-rc.1
#
# Run it from a checkout with the release tags. It runs jobs in the server's own
# process (HAAS_SANDBOX=here) with a fixture harness that calls no model, so it
# needs Go and git and no container runtime.
set -eu

[ "$#" -ge 1 ] || { echo "usage: $0 <release> [<release>...]" >&2; exit 2; }
root=$(git rev-parse --show-toplevel)
work=$(mktemp -d)
pid=""
stop() { [ -n "$pid" ] && kill "$pid" 2>/dev/null && wait "$pid" 2>/dev/null || true; pid=""; }
cleanup() { stop; for tree in "$work"/src-*; do [ -d "$tree" ] && git -C "$root" worktree remove --force "$tree" 2>/dev/null; done; rm -rf "$work"; }
trap cleanup EXIT

port=$((20000 + $$ % 20000))
address="127.0.0.1:$port"
new="$work/haas-new"
(cd "$root" && go build -o "$new" ./cmd/haas)

# start runs the binary given as a server on the data in $data, and waits for it.
start() {
  HAAS_PASSWORD=upgrade-check HAAS_ADDR="$address" HAAS_SANDBOX=here \
    HAAS_JOBS="$data/jobs" HAAS_SESSIONS="$data/sessions" HAAS_SETUPS="$data/setups" HAAS_INTEGRATIONS="$data/integrations" HAAS_PRESETS="$data/presets" \
    HAAS_COMPARISONS="$data/comparisons" HAAS_CHANGES="$data/changes/changes.jsonl" \
    HAAS_LOCAL_REPOS="$data/repo" \
    HAAS_SIGNING_KEY="$data/signing/upgrade.key" HAAS_KEYRING="$data/keyring" \
    "$1" serve > "$data/serve.log" 2>&1 &
  pid=$!
  tries=0
  until curl -fsS "http://$address/healthz" >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ "$tries" -gt 60 ] || ! kill -0 "$pid" 2>/dev/null; then
      echo "the server did not come up:" >&2; tail -n 20 "$data/serve.log" >&2; exit 1
    fi
    sleep 1
  done
}

# reads checks the job is served and verifies, as the binary given.
reads() {
  curl -fsS -c "$data/jar" -o /dev/null -d password=upgrade-check "http://$address/login" -w '' || true
  page=$(curl -fsS -b "$data/jar" "http://$address/jobs/$id") || { echo "$1: the job page did not load" >&2; exit 1; }
  echo "$page" | grep -q "make add add" || { echo "$1: the job page does not show the job" >&2; exit 1; }
  HAAS_KEYRING="$data/keyring" "$2" verify "$data/jobs/$id" > /dev/null || { echo "$1: verify does not hold" >&2; exit 1; }
  echo "  $1: the job page loads and its record verifies"
}

for ref in "$@"; do
  echo "== from $ref"
  git -C "$root" rev-parse --verify --quiet "$ref^{commit}" >/dev/null || { echo "no release $ref in this checkout: fetch tags" >&2; exit 1; }
  git -C "$root" worktree add --detach --quiet "$work/src-$ref" "$ref"
  old="$work/haas-$ref"
  (cd "$work/src-$ref" && go build -o "$old" ./cmd/haas)
  data="$work/data-$ref"
  mkdir -p "$data/signing" "$data/keyring" "$data/repo" "$data/harnesses"
  "$old" keygen upgrade -dir "$data/signing" >/dev/null
  cp "$data/signing/upgrade.pub" "$data/keyring/"

  # A repository whose test fails until the job fixes it, and a harness that
  # makes the change.
  (cd "$data/repo" && printf 'add() {\n  echo $(($1 - $2))\n}\n' > adder.sh \
    && printf '#!/bin/sh\n. ./adder.sh\n[ "$(add 2 3)" = 5 ] && exit 0\nexit 1\n' > test.sh \
    && git init -q . && git add -A && git -c user.email=a@b -c user.name=x commit -qm adder)
  cat > "$data/harnesses/fixture.json" <<'JSON'
{"name":"fixture","program":"sh","argv":["-c","sed -i.bak s/-/+/ adder.sh; true"],"needs":"nothing","stream":"plain"}
JSON
  HAAS_HARNESSES="$data/harnesses" "$old" new "$data/job" -repo "$data/repo" -brief "make add add" -harness fixture -- sh test.sh >/dev/null

  echo "  $ref makes a job"
  start "$old"
  id=$(HAAS_SERVER="http://$address" HAAS_PASSWORD=upgrade-check "$old" submit "$data/job" | head -n 1)
  tries=0
  until [ -f "$data/jobs/$id/out/manifest.json.sig" ] || [ -f "$data/jobs/$id/out/manifest.sig" ]; do
    tries=$((tries + 1)); [ "$tries" -le 90 ] || { echo "$ref: the job did not finish" >&2; tail -n 20 "$data/serve.log" >&2; exit 1; }
    sleep 1
  done
  sleep 1
  stop
  HAAS_KEYRING="$data/keyring" "$old" verify "$data/jobs/$id" >/dev/null

  echo "  rehearsing the upgrade on a copy"
  HAAS_KEYRING="$data/keyring" "$root/deploy/rehearse-upgrade.sh" "$new" "$data" "$ref" > "$work/rehearsal.log" 2>&1 \
    || { cat "$work/rehearsal.log" >&2; echo "$ref: the rehearsal on a copy failed" >&2; exit 1; }
  tail -n 1 "$work/rehearsal.log" | sed 's/^/  /'
  # A rehearsal that cannot fail proves nothing: a job whose record was edited has to be reported.
  rm -rf "$work/tampered" && cp -R "$data" "$work/tampered"
  echo "edited after it was signed" >> "$work/tampered/jobs/$id/out/transcript.log"
  if HAAS_KEYRING="$data/keyring" "$root/deploy/rehearse-upgrade.sh" "$new" "$work/tampered" > "$work/tampered.log" 2>&1; then
    echo "$ref: the rehearsal passed a job whose record had been edited" >&2; exit 1
  fi
  echo "  and it reports a record that was edited"

  echo "  upgrading to this checkout"
  export HAAS_JOBS="$data/jobs" HAAS_SESSIONS="$data/sessions" HAAS_SETUPS="$data/setups" HAAS_INTEGRATIONS="$data/integrations" \
    HAAS_COMPARISONS="$data/comparisons" HAAS_CHANGES="$data/changes/changes.jsonl"
  "$new" data check
  start "$new"
  reads "this checkout" "$new"
  stop
  "$new" data status

  echo "  rolling back to $ref"
  "$new" data rollback -to "$ref"
  start "$old"
  reads "$ref, after the rollback" "$old"
  stop
  unset HAAS_JOBS HAAS_SESSIONS HAAS_SETUPS HAAS_COMPARISONS HAAS_CHANGES
  echo "== $ref: upgrade and rollback hold"
done
