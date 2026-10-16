#!/bin/sh
# Builds the gusto program for one system and packs it: a .tar.gz for macOS and Linux, a .zip for Windows, named for the release,
# the system and the processor. The release workflow runs it for six targets, and a person can run it to build their own. It needs
# no C compiler, so every target builds on any machine.
#
#   deploy/server/build.sh v0.0.1-alpha.6 darwin arm64 dist
set -eu
tag=${1:?the release, such as v0.0.1-alpha.6, or dev}
os=${2:?linux, darwin or windows}
arch=${3:?amd64 or arm64}
out=${4:-dist}

exe=gusto
[ "$os" = windows ] && exe=gusto.exe
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
root=$(git rev-parse --show-toplevel)

cp "$root/LICENSE" "$root/NOTICE" "$work/"

(cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.release=$tag" -o "$work/$exe" ./cmd/gusto)

mkdir -p "$out"
archive="gusto_${tag}_${os}_${arch}"
if [ "$os" = windows ]; then
  (cd "$work" && zip -q -9 "$archive.zip" "$exe" LICENSE NOTICE) && mv "$work/$archive.zip" "$out/"
  echo "$out/$archive.zip"
else
  chmod 0755 "$work/$exe"
  # macOS's tar adds its own file attributes, which tar on Linux warns about.
  COPYFILE_DISABLE=1 tar --no-xattrs -C "$work" -czf "$out/$archive.tar.gz" "$exe" LICENSE NOTICE
  echo "$out/$archive.tar.gz"
fi
