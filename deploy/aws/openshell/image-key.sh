#!/usr/bin/env bash
# What names the runner image: the harnesses, the Dockerfile and image/. The gusto program inside it
# belongs to the container backend, and a sandbox on a gateway runs nothing of it, so a change to
# the Go sources does not make a new image. It prints the key.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
{ "$root/deploy/aws/openshell/harnesses.sh"; cat "$root/Dockerfile"; (cd "$root" && git ls-files -z image | xargs -0 shasum -a 256); } | shasum -a 256 | cut -c1-16
