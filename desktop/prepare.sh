#!/bin/sh
# Builds the programs the app carries, haas-helper and haas, named for the platform they are for, in src-tauri/binaries, which is where
# the app's bundle looks for them. With no argument they are for this machine; ./prepare.sh aarch64-apple-darwin names another. The app
# starts the helper with its own icon turned off, so neither program needs a C compiler.
set -eu
triple=${1:-$(rustc -vV | sed -n 's/^host: //p')}
case "$triple" in
  aarch64-apple-darwin)       os=darwin arch=arm64 ;;
  x86_64-apple-darwin)        os=darwin arch=amd64 ;;
  x86_64-pc-windows-msvc)     os=windows arch=amd64 ;;
  aarch64-pc-windows-msvc)    os=windows arch=arm64 ;;
  x86_64-unknown-linux-gnu)   os=linux arch=amd64 ;;
  aarch64-unknown-linux-gnu)  os=linux arch=arm64 ;;
  *) echo "no helper is built for $triple" >&2; exit 2 ;;
esac
tag=${HAAS_VERSION:-dev}
root=$(git rev-parse --show-toplevel)
out="$root/desktop/src-tauri/binaries"
mkdir -p "$out"
ext=""
[ "$os" = windows ] && ext=".exe"
flags="-s -w -X github.com/getlatentic/haas/internal/helper/cli.Version=$tag"
[ "$os" = windows ] && flags="$flags -H=windowsgui"
(cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "$flags" -o "$out/haas-helper-$triple$ext" ./cmd/haas-helper)
echo "$out/haas-helper-$triple$ext"
(cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.release=$tag" -o "$out/haas-$triple$ext" ./cmd/haas)
echo "$out/haas-$triple$ext"
