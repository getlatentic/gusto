#!/usr/bin/env bash
# Builds static/app.css, the whole app's stylesheet, from the files in this folder: the gusto tokens mapped to Tailwind's names,
# a base, the few shared components, Markdown's typography and each page's own.
# Needs the tailwindcss standalone CLI (brew install tailwindcss), which carries @tailwindcss/typography.
# Sizes are in rem, so the pixels Tailwind writes for hairlines and shadows are turned into rem.
#   internal/web/tailwind/build.sh            build once
#   internal/web/tailwind/build.sh --watch    rebuild when a template or a file here changes
set -euo pipefail
cd "$(dirname "$0")"
if [ "${1:-}" = "--watch" ]; then
  exec tailwindcss -i app.css -o ../static/app.css --watch
fi
tailwindcss -i app.css -o ../static/app.css --minify
perl -pi -e 's/(?<![\w.])(\d*\.?\d+)px/($1\/16)."rem"/ge' ../static/app.css
