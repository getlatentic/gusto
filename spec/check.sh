#!/usr/bin/env bash
# Model-checks every spec here with TLC. A config named *.before-*.cfg models the code before a fix, and must fail: each shows
# that its spec finds the bug the fix closed. Every other config must pass. TLA2TOOLS names a tla2tools.jar to use; without
# it the pinned release is fetched once into the cache and its checksum checked.
set -euo pipefail
cd "$(dirname "$0")"
version=v1.7.4
sum=936a262061c914694dfd669a543be24573c45d5aa0ff20a8b96b23d01e050e88
jar="${TLA2TOOLS:-${XDG_CACHE_HOME:-$HOME/.cache}/gusto/tla2tools-$version.jar}"
if [[ ! -f "$jar" ]]; then
  mkdir -p "$(dirname "$jar")"
  curl -fsSL --max-time 120 -o "$jar.part" "https://github.com/tlaplus/tlaplus/releases/download/$version/tla2tools.jar"
  mv "$jar.part" "$jar"
fi
if [[ -z "${TLA2TOOLS:-}" ]] && ! echo "$sum  $jar" | shasum -a 256 -c - >/dev/null; then
  rm -f "$jar"
  echo "tla2tools.jar $version does not have the pinned checksum" >&2
  exit 1
fi
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
failed=0
for config in *.cfg; do
  spec="${config%%.*}.tla"
  set +e
  java -XX:+UseParallelGC -cp "$jar" tlc2.TLC -config "$config" -workers auto -metadir "$scratch/${config%.cfg}" -cleanup \
    "$spec" >"$scratch/out" 2>&1
  status=$?
  set -e
  # TLC exits 12 when an invariant or action property is broken, and 13 when a liveness property is.
  if [[ "$config" == *.before-*.cfg ]]; then
    if [[ $status == 12 || $status == 13 ]]; then
      echo "ok   $config: $(grep -m1 -E 'is violated' "$scratch/out")"
    else
      echo "FAIL $config: TLC exited $status, but this config must find the bug"
      failed=1
    fi
  elif [[ $status == 0 ]]; then
    echo "ok   $config: $(grep -m1 -E '^[0-9]+ states generated' "$scratch/out")"
  else
    echo "FAIL $config: TLC exited $status"
    cat "$scratch/out"
    failed=1
  fi
done
exit $failed
