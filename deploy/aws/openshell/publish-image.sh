#!/usr/bin/env bash
# Puts the runner image in the demo's bucket under its key, when it is not there: it is built on an x86 host made for the
# purpose and removed after, since a runner that is not x86 cannot build it. A change to image/, the Dockerfile or the harnesses
# makes a new key, and every host that starts after it needs the image of that key. About ten minutes of an m8i.xlarge.
set -euo pipefail
source "$(dirname "$0")/env.sh"
bucket="$(./demo-store.sh)"
[[ "$bucket" =~ ^gusto-poc-demo-[0-9]{12}$ ]] || { echo "demo-store.sh did not give a bucket name: $bucket" >&2; exit 1; }
key="$(./image-key.sh)"
if aws s3api head-object --bucket "$bucket" --key "image/$key.tar.gz" >/dev/null 2>&1; then
  echo "the image $key is in the bucket"
  exit 0
fi
export TF_VAR_demo_bucket="$bucket"
# The host is removed however this ends, and what removing it prints does not change how this ended.
trap 'status=$?; ./down.sh || true; exit $status' EXIT
./up.sh
address="$(terraform output -raw address)"
./image.sh runner-all-rootfs "$(./harnesses.sh)"
ssh "${ssh_options[@]}" "ubuntu@$address" "AWS_REGION=$AWS_REGION /snap/bin/aws s3 cp ~/e2e/runner-all-rootfs.tar.gz s3://$bucket/image/$key.tar.gz --only-show-errors"
echo "made the image $key"
