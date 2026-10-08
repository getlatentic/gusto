#!/bin/sh
# Builds haas-helper for one system and packs it: a .tar.gz for macOS and Linux, a .zip for Windows, named for the release, the
# system and the processor. The release workflow runs it for each of six targets, and a person can run it to build their own.
#
#   deploy/helper/build.sh v0.0.1-alpha.5 linux amd64 dist
#
# The program is built with no C compiler and no paths of the machine it was built on, so the same source gives the same bytes.
set -eu
tag=${1:?the release, such as v0.0.1-alpha.5, or dev}
os=${2:?linux, darwin or windows}
arch=${3:?amd64 or arm64}
out=${4:-dist}

name=haas-helper
exe=$name
[ "$os" = windows ] && exe=$name.exe
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
root=$(git rev-parse --show-toplevel)

(cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
  -ldflags "-s -w -X github.com/getlatentic/haas/internal/helper/cli.Version=$tag" -o "$work/$exe" ./cmd/haas-helper)

mkdir -p "$out"
archive="${name}_${tag}_${os}_${arch}"
if [ "$os" = windows ]; then
  (cd "$work" && zip -q -9 "$archive.zip" "$exe") && mv "$work/$archive.zip" "$out/"
  echo "$out/$archive.zip"
else
  chmod 0755 "$work/$exe"
  tar -C "$work" -czf "$out/$archive.tar.gz" "$exe"
  echo "$out/$archive.tar.gz"
fi
