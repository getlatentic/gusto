#!/bin/sh
# Builds the web app into internal/web/spa/dist, which the gusto binary embeds. The built files are not committed, so every script
# that builds a gusto to run or to ship runs this first. It installs the packages only when package-lock.json is newer than the
# install.
set -eu
cd "$(dirname "$0")"
[ node_modules/.package-lock.json -nt package-lock.json ] || npm ci --no-audit --no-fund
npm run build --silent
