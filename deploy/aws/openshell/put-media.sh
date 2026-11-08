#!/usr/bin/env bash
# Puts files where the front door serves them, at https://<demo name>/media/<file name>: the bucket's media/, which the
# front door copies at its next upgrade. The landing page's video is one, named in demo.env as DEMO_VIDEO_URL.
#   ./put-media.sh gusto-demo.mp4
set -euo pipefail
source "$(dirname "$0")/env.sh"
[ $# -gt 0 ] || { echo "usage: $0 file..." >&2; exit 2; }
bucket="$(./demo-store.sh)"
[[ "$bucket" =~ ^gusto-poc-demo-[0-9]{12}$ ]] || { echo "demo-store.sh did not give a bucket name: $bucket" >&2; exit 1; }
for file in "$@"; do
  aws s3 cp "$file" "s3://$bucket/media/$(basename "$file")" --only-show-errors
  echo "put media/$(basename "$file")"
done
