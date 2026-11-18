#!/usr/bin/env bash
# Puts files where the front door serves them, at https://<cloud domain>/media/<file name>: the bucket's media/, which the
# front door copies at its next upgrade.
#   ./put-media.sh gusto-demo.mp4
set -euo pipefail
source "$(dirname "$0")/env.sh"
[ $# -gt 0 ] || { echo "usage: $0 file..." >&2; exit 2; }
bucket="$(./cloud-store.sh)"
[[ "$bucket" =~ ^gusto-store-[0-9]{12}$ ]] || { echo "cloud-store.sh did not give a bucket name: $bucket" >&2; exit 1; }
for file in "$@"; do
  aws s3 cp "$file" "s3://$bucket/media/$(basename "$file")" --only-show-errors
  echo "put media/$(basename "$file")"
done
