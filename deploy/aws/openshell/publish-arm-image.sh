#!/usr/bin/env bash
# Puts the Arm runner image in the store bucket under its key with -arm64 after it, when it is not there. It is built here, with
# Docker, on an Arm machine such as a Mac, since building it anywhere else needs emulation. The gusto cloud that stays up is Arm and
# runs its agents in containers, so it needs the archive Docker loads and not the root file system OpenShell makes a sandbox from.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
here="$root/deploy/aws/openshell"
bucket="${BUCKET:-$("$here/cloud-store.sh")}"
[[ "$bucket" =~ ^gusto-store-[0-9]{12}$ ]] || { echo "the store bucket is not named as cloud-store.sh makes it: $bucket" >&2; exit 1; }
object="image/$("$here/image-key.sh")-arm64.docker.tar.gz"
if aws s3api head-object --bucket "$bucket" --key "$object" >/dev/null 2>&1; then
  echo "$object is in the bucket"
  exit 0
fi
case "$(uname -m)" in arm64 | aarch64) ;; *) echo "this machine is $(uname -m), and the Arm image is built on an Arm one" >&2; exit 1 ;; esac
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# A host loads the archive and tags gusto-runner:latest with its key, so the archive holds that one tag.
docker build --platform linux/arm64 --build-arg "HARNESSES=$("$here/harnesses.sh")" -t gusto-runner:latest "$root"
docker save gusto-runner:latest | gzip -1 > "$work/image.docker.tar.gz"
aws s3 cp "$work/image.docker.tar.gz" "s3://$bucket/$object" --only-show-errors
echo "made $object: $(du -h "$work/image.docker.tar.gz" | cut -f1)"
