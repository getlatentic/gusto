#!/bin/sh
# Makes a folder that holds a release as the install scripts read it, from this checkout: gusto for each system named, its
# checksums, and the two install scripts. Serve the folder, and point GUSTO_RELEASE_URL at it, to test an install on a machine
# with no release to download, such as a clean virtual machine. Nothing in it is signed, so a machine with cosign refuses it.
#
#   deploy/install/test-release.sh v0.0.0-test dist/test-release darwin/arm64 linux/arm64 windows/arm64
#   python3 -m http.server -d dist/test-release 8000
set -eu
tag=${1:?the release name to give the build, such as v0.0.0-test}
out=${2:?the folder to make}
shift 2
[ "$#" -gt 0 ] || set -- darwin/arm64 linux/arm64 windows/arm64
here=$(cd "$(dirname "$0")" && pwd)

rm -rf "$out"
mkdir -p "$out"
for target in "$@"; do
  "$here/../server/build.sh" "$tag" "${target%/*}" "${target#*/}" "$out" >/dev/null
done
(cd "$out" && if command -v sha256sum >/dev/null 2>&1; then sha256sum gusto_*; else shasum -a 256 gusto_*; fi > SHA256SUMS)
cp "$here/install.sh" "$here/install.ps1" "$out/"
ls "$out"
