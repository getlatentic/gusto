#!/usr/bin/env bash
# Builds static/operator.css, the operator page's stylesheet: the gusto tokens mapped to Tailwind's names, a base, and the
# utilities its template uses. Needs the tailwindcss standalone CLI (brew install tailwindcss).
# Sizes are in rem, so the pixels Tailwind writes for hairlines and shadows are turned into rem.
#   internal/web/tailwind/build.sh            build once
#   internal/web/tailwind/build.sh --watch    rebuild when the template or a file here changes
set -euo pipefail
cd "$(dirname "$0")"
if [ "${1:-}" = "--watch" ]; then
  exec tailwindcss -i operator.css -o ../static/operator.css --watch
fi
tailwindcss -i operator.css -o ../static/operator.css --minify
perl -pi -e 's/(?<![\w.])(\d*\.?\d+)px/($1\/16)."rem"/ge' ../static/operator.css
