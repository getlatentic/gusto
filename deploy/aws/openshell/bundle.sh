#!/usr/bin/env bash
# Makes what a host configures itself from: the Linux gusto and its DNS program, the
# playbooks and what they copy, and the key of the runner image it needs. bundle.tgz and its
# checksum are written to the directory given, which CI puts in the store bucket.
set -euo pipefail
out="${1:?the directory to write bundle.tgz in}"
root="$(git rev-parse --show-toplevel)"
here="$root/deploy/aws/openshell"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
mkdir -p "$out" "$stage/.state" "$stage/probe" "$stage/e2e"
"$root/web/build.sh"
# A host of either architecture is made from the one bundle: its programs are in .state for x86-64 and in .state/arm64 for Arm.
mkdir -p "$stage/.state/arm64"
(cd "$root" && for program in gusto:./cmd/gusto gusto-dns:./deploy/aws/openshell/host/dns; do
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$stage/.state/${program%%:*}" "${program#*:}"
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o "$stage/.state/arm64/${program%%:*}" "${program#*:}"
done)
cp -R "$here/ansible" "$stage/ansible"
cp "$here/probe/inside.sh" "$stage/probe/"
cp "$root/deploy/aws/platform/user-data.sh" "$stage/user-data.sh"
cp "$here/e2e/credentials.sh" "$stage/e2e/"
"$here/image-key.sh" > "$stage/image.key"
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$out/bundle.tgz" -C "$stage" . 2>/dev/null || tar -czf "$out/bundle.tgz" -C "$stage" .
(cd "$out" && if command -v sha256sum >/dev/null; then sha256sum bundle.tgz; else shasum -a 256 bundle.tgz; fi > bundle.tgz.sha256)
echo "$(cat "$stage/image.key") image key; $(du -h "$out/bundle.tgz" | cut -f1) bundle"
