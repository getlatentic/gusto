#!/usr/bin/env bash
# Makes what a demo host configures itself from: the Linux haas and its DNS program, the
# playbooks and what they copy, and the key of the runner image it needs. bundle.tgz and its
# checksum are written to the directory given, which CI puts in the store bucket.
set -euo pipefail
out="${1:?the directory to write bundle.tgz in}"
root="$(git rev-parse --show-toplevel)"
here="$root/deploy/aws/openshell"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
mkdir -p "$out" "$stage/.state" "$stage/probe" "$stage/e2e"
(cd "$root" && for program in haas:./cmd/haas haas-dns:./deploy/aws/openshell/demo/dns; do
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$stage/.state/${program%%:*}" "${program#*:}"
done)
cp -R "$here/ansible" "$stage/ansible"
cp "$here/probe/inside.sh" "$stage/probe/"
cp "$here/e2e/credentials.sh" "$stage/e2e/"
"$here/image-key.sh" > "$stage/image.key"
COPYFILE_DISABLE=1 tar --no-xattrs -czf "$out/bundle.tgz" -C "$stage" . 2>/dev/null || tar -czf "$out/bundle.tgz" -C "$stage" .
(cd "$out" && { command -v sha256sum >/dev/null && sha256sum bundle.tgz || shasum -a 256 bundle.tgz; } > bundle.tgz.sha256)
echo "$(cat "$stage/image.key") image key; $(du -h "$out/bundle.tgz" | cut -f1) bundle"
