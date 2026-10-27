#!/usr/bin/env bash
# What names the runner image: the harnesses, their pins and installer, the Dockerfile, image/, and the part of the gusto program
# inside it that a sandbox on a gateway runs: sandbox-run, which holds a room's turn to its Landlock policy. The rest of the Go
# sources belong to the container backend and do not make a new image. It prints the key.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
{ "$root/deploy/aws/openshell/harnesses.sh"; cat "$root/Dockerfile" "$root/pkg/harness/pins.json" "$root/deploy/runner/install.mjs"; (cd "$root" && git ls-files -z image internal/landlock cmd/gusto/sandboxrun.go | xargs -0 shasum -a 256); } | shasum -a 256 | cut -c1-16
