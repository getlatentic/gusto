#!/usr/bin/env bash
# The harnesses the runner image carries, as name=package on one line.
grep -v '^#' "$(git rev-parse --show-toplevel)/deploy/runner/harnesses.txt" | xargs
