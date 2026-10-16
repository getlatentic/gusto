#!/usr/bin/env bash
# Makes recording/gusto-demo.mp4 and recording/poster.jpg from what record.mjs recorded, playing the waits it marked at
# SPEED times their speed and leaving the rest as it was.
set -euo pipefail
cd "$(dirname "$0")/recording"
speed=${SPEED:-5}
filter="$(python3 - "$speed" "${W:-iw}" "${H:-ih}" <<'PY'
import json, sys
speed = float(sys.argv[1])
w, h = sys.argv[2], sys.argv[3]
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
print(";".join(parts) + ";" + "".join(f"[v{i}]" for i in range(n)) + f"concat=n={n}:v=1:a=0,scale={w}:{h},format=yuv420p[out]")
PY
)"
ffmpeg -y -loglevel error -i raw.mp4 -filter_complex "$filter" -map "[out]" -c:v libx264 -crf "${CRF:-16}" -preset medium -r 30 -movflags +faststart gusto-demo.mp4
ffmpeg -y -loglevel error -ss 2 -i gusto-demo.mp4 -frames:v 1 -q:v 4 poster.jpg
# Where each caption falls in the shortened video, for edit/.
python3 - "$speed" <<'PY'
import json, sys
speed = float(sys.argv[1])
cuts = sorted((s["from"], s["to"]) for s in json.load(open("segments.json")) if s["to"] - s["from"] > 3)
def encoded(t):
    out, at = 0.0, 0.0
    for a, b in cuts:
        if t <= a: return out + (t - at)
        out += a - at
        if t <= b: return out + (t - a) / speed
        out += (b - a) / speed
        at = b
    return out + (t - at)
marks = json.load(open("marks.json"))
json.dump([{**m, "t": round(encoded(m["t"]), 3), **({"cut": round(encoded(m["cut"]), 3)} if "cut" in m else {})} for m in marks], open("timeline.json", "w"), indent=1)
PY
ls -lh gusto-demo.mp4 poster.jpg
ffprobe -v error -show_entries format=duration -of csv=p=0 gusto-demo.mp4
