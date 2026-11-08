#!/usr/bin/env bash
# What names the runner image: the harnesses, their pins and installer, the Dockerfile, image/, and the parts of the gusto program
# inside it that run in a box. On a gateway that is boxed, which starts an agent with its run's forwarder (internal/runtoken), and
# sandbox-run and internal/landlock, which hold a room's turn to its Landlock policy. In a container it is `gusto inside`: main.go
# and internal/runner with every package of this module it imports, which Go lists. It prints the key.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
inside="$(cd "$root" && go list -deps -f '{{if not .Standard}}{{.Dir}}{{end}}' ./internal/runner | grep "^$root/" | sed "s|^$root/||" | sort -u)"
{ "$root/deploy/aws/openshell/harnesses.sh"; cat "$root/Dockerfile" "$root/pkg/harness/pins.json" "$root/deploy/runner/install.mjs"; (cd "$root" && git ls-files -z image internal/landlock internal/runtoken cmd/gusto/sandboxrun.go 'cmd/gusto/boxed*.go' cmd/gusto/main.go $inside | xargs -0 shasum -a 256); } | shasum -a 256 | cut -c1-16
