#!/usr/bin/env bash
# Edits the recording into the video the landing page shows: the camera moves in on what each caption is about, captions are
# drawn over it, and a title and an end card are added. Run record.mjs with CAPTIONS=off and SCALE=1.5, then ../encode.sh with
# W=1920 H=1080, first. Writes out/demo.mp4 and, with INSTALL=1, copies it and a poster to internal/cloud/static.
set -euo pipefail
cd "$(dirname "$0")"
root="$(git rev-parse --show-toplevel)"
mkdir -p public out
cp ../recording/haas-demo.mp4 public/footage.mp4
cp ../recording/timeline.json public/timeline.json
cp "$root/internal/web/static/mark.png" public/mark.png
[ -d node_modules ] || npm install --no-audit --no-fund
npx tsc --noEmit
npx remotion render src/index.ts Demo out/demo.mp4 --crf=26
ffmpeg -y -loglevel error -ss 1.5 -i out/demo.mp4 -frames:v 1 -q:v 4 out/poster.jpg
if [ "${INSTALL:-}" = 1 ]; then
  cp out/demo.mp4 "$root/internal/cloud/static/demo.mp4"
  cp out/poster.jpg "$root/internal/cloud/static/demo-poster.jpg"
fi
ls -lh out/demo.mp4
