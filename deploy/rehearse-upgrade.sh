#!/bin/sh
# Rehearses an upgrade on a copy of your data, so a migration is not first tried on the real thing.
#
#   deploy/rehearse-upgrade.sh <the new haas binary> <your HAAS_DATA directory> [<a release to roll back to>]
#
# The directory is copied and never changed. On the copy, the script has the new binary check what it would migrate, starts
# it as a server, reads the job pages, and runs `haas verify` on every job. With a release named, it then rolls the copy back
# to that release's formats. It runs jobs in the server's own process and runs none, so it needs the binary and curl, and no
# container runtime. A binary is built from a checkout with `go build -o haas ./cmd/haas`.
#
# It reads the signing keys' public halves from <data>/keys when that directory exists, or from HAAS_KEYRING. A job signed
# with a key it cannot find is reported, so point HAAS_KEYRING at the keyring the server uses.
#
# It exits 0 when the copy upgrades, the pages load and every job verifies, and 1 otherwise, saying which.
set -eu

[ "$#" -ge 2 ] || { echo "usage: $0 <new haas binary> <HAAS_DATA directory> [<release to roll back to>]" >&2; exit 2; }
binary=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
source_dir=$2
release=${3:-}
[ -x "$binary" ] || { echo "$1 is not a program" >&2; exit 2; }
[ -d "$source_dir/jobs" ] || { echo "$source_dir has no jobs directory: give the directory HAAS_DATA names" >&2; exit 2; }

work=$(mktemp -d)
pid=""
stop() { [ -n "$pid" ] && kill "$pid" 2>/dev/null && wait "$pid" 2>/dev/null || true; pid=""; }
trap 'stop; rm -rf "$work"' EXIT
data="$work/data"
echo "copying $source_dir to a scratch directory"
cp -R "$source_dir" "$data"

export HAAS_JOBS="$data/jobs" HAAS_SESSIONS="$data/sessions" HAAS_SETUPS="$data/setups" \
  HAAS_COMPARISONS="$data/comparisons" HAAS_CHANGES="$data/changes/changes.jsonl"
[ -z "${HAAS_KEYRING:-}" ] && [ -d "$data/keys" ] && export HAAS_KEYRING="$data/keys"

echo "== what the new binary would migrate"
"$binary" data check

port=$((20000 + $$ % 20000))
address="127.0.0.1:$port"
echo "== starting it on the copy"
HAAS_PASSWORD=rehearsal HAAS_ADDR="$address" HAAS_SANDBOX=here HAAS_LOCAL_REPOS="$data/smoke" \
  "$binary" serve > "$work/serve.log" 2>&1 &
pid=$!
tries=0
until curl -fsS "http://$address/healthz" >/dev/null 2>&1; do
  tries=$((tries + 1))
  if [ "$tries" -gt 120 ] || ! kill -0 "$pid" 2>/dev/null; then
    echo "FAIL: the server did not start on the copy:" >&2; tail -n 20 "$work/serve.log" >&2; exit 1
  fi
  sleep 1
done
curl -fsS -c "$work/jar" -o /dev/null -d password=rehearsal "http://$address/login" -w '' || true

jobs=0; loaded=0; held=0; broken=0
for dir in "$data"/jobs/*/; do
  [ -f "${dir}job.json" ] || continue
  id=$(basename "$dir")
  jobs=$((jobs + 1))
  if curl -fsS -b "$work/jar" -o /dev/null "http://$address/jobs/$id"; then loaded=$((loaded + 1)); else echo "  the page of $id did not load" >&2; broken=$((broken + 1)); fi
  if [ ! -f "${dir}out/manifest.json" ]; then continue; fi
  if "$binary" verify "$dir" > "$work/verify.out" 2>&1; then held=$((held + 1)); else echo "  $id does not verify: $(tail -n 1 "$work/verify.out")" >&2; broken=$((broken + 1)); fi
done
stop
echo "== $jobs jobs: $loaded pages loaded, $held records verified, $broken problems"
"$binary" data status

if [ -n "$release" ]; then
  echo "== rolling the copy back to $release"
  "$binary" data rollback -to "$release"
fi
[ "$broken" = 0 ] || { echo "FAIL: the upgrade is not safe to run on this data until the problems above are explained" >&2; exit 1; }
echo "the copy upgrades, its pages load and its records verify. Your data was not changed."
