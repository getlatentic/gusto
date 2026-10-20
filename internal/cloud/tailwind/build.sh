#!/usr/bin/env bash
# Builds static/cloud.css, the front door's whole stylesheet, from app.css and the templates.
# Needs the tailwindcss standalone CLI (brew install tailwindcss).
# Sizes are in rem, so the pixels Tailwind writes for hairlines and shadows are turned into rem.
#   internal/cloud/tailwind/build.sh            build once
#   internal/cloud/tailwind/build.sh --watch    rebuild when a template or app.css changes
set -euo pipefail
cd "$(dirname "$0")"
if [ "${1:-}" = "--watch" ]; then
  exec tailwindcss -i app.css -o ../static/cloud.css --watch
fi
tailwindcss -i app.css -o ../static/cloud.css --minify
perl -pi -e 's/(?<![\w.])(\d*\.?\d+)px/($1\/16)."rem"/ge' ../static/cloud.css
