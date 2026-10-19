#!/usr/bin/env bash
# Builds static/prose.css: the typography Tailwind gives rendered Markdown, mapped onto the gusto tokens.
# Needs the tailwindcss standalone CLI (brew install tailwindcss), which carries @tailwindcss/typography.
# Sizes are in rem, so the pixels the plugin writes for hairlines and shadows are turned into rem.
set -euo pipefail
cd "$(dirname "$0")"
tailwindcss -i prose.css -o ../static/prose.css --minify
perl -pi -e 's/(?<![\w.])(\d+)px/($1\/16)."rem"/ge' ../static/prose.css
