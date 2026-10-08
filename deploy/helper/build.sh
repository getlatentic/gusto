#!/bin/sh
# Builds haas-helper for one system and packs it: a .tar.gz for macOS and Linux, a .zip for Windows, named for the release, the
# system and the processor. The release workflow runs it for each of six targets, and a person can run it to build their own.
#
#   deploy/helper/build.sh v0.0.1-alpha.5 linux amd64 dist
#
# Linux and Windows programs are built with no C compiler. A macOS program is built on macOS with its C compiler, because the
# menu bar icon is drawn through the system's libraries; built anywhere else it has no icon. No build holds a path of the
# machine it was built on.
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

cgo=0
[ "$os" = darwin ] && [ "$(uname -s)" = Darwin ] && cgo=1

# A Windows program is built with no window of its own, so that starting it at login opens no black one: it attaches to the terminal
# it was started from, when it was started from one (cmd/haas-helper/console_windows.go).
flags="-s -w -X github.com/getlatentic/haas/internal/helper/cli.Version=$tag"
[ "$os" = windows ] && flags="$flags -H=windowsgui"

(cd "$root" && CGO_ENABLED=$cgo GOOS=$os GOARCH=$arch go build -trimpath -ldflags "$flags" -o "$work/$exe" ./cmd/haas-helper)

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
