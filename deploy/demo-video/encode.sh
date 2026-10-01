#!/usr/bin/env bash
# Makes recording/haas-demo.mp4 and recording/poster.jpg from what record.mjs recorded, playing the waits it marked at
# SPEED times their speed and leaving the rest as it was.
set -euo pipefail
cd "$(dirname "$0")/recording"
speed=${SPEED:-12}
filter="$(python3 - "$speed" <<'PY'
import json, sys
speed = float(sys.argv[1])
cuts = sorted((s["from"], s["to"]) for s in json.load(open("segments.json")) if s["to"] - s["from"] > 3)
parts, at, n = [], 0.0, 0
def add(a, b, fast):
    global n
    if b - a < 0.05: return
    factor = f"setpts=(PTS-STARTPTS)/{speed}" if fast else "setpts=PTS-STARTPTS"
    parts.append(f"[0:v]trim=start={a:.3f}:end={b:.3f},{factor}[v{n}]")
    n += 1
for a, b in cuts:
    add(at, a, False); add(a, b, True); at = b
add(at, 1e6, False)
print(";".join(parts) + ";" + "".join(f"[v{i}]" for i in range(n)) + f"concat=n={n}:v=1:a=0,scale=1280:720,format=yuv420p[out]")
PY
)"
ffmpeg -y -loglevel error -i haas-demo.webm -filter_complex "$filter" -map "[out]" -c:v libx264 -crf 27 -preset slow -r 25 -movflags +faststart haas-demo.mp4
ffmpeg -y -loglevel error -ss 2 -i haas-demo.mp4 -frames:v 1 -q:v 4 poster.jpg
ls -lh haas-demo.mp4 poster.jpg
ffprobe -v error -show_entries format=duration -of csv=p=0 haas-demo.mp4
