#!/bin/sh
# Installs haas-helper on macOS or Linux from a release: downloads the package for this system, checks it against the release's
# checksums, and checks the checksums' signature when cosign is installed. The releases are private, so it uses the GitHub
# command line (https://cli.github.com), signed in as somebody who may read them.
#
#   deploy/helper/install.sh                       # the newest release, this alpha included
#   deploy/helper/install.sh v0.0.1-alpha.5
#   HAAS_HELPER_DIR=/usr/local/bin deploy/helper/install.sh      # where to put it; ~/.local/bin by default
#   HAAS_REQUIRE_COSIGN=1 deploy/helper/install.sh               # refuse to go on without checking the signature
set -eu

repo=${HAAS_REPO:-getlatentic/haas}
tag=${1:-latest}
dir=${HAAS_HELPER_DIR:-$HOME/.local/bin}

fail() { echo "install: $*" >&2; exit 1; }
command -v gh >/dev/null 2>&1 || fail "this needs the GitHub command line, signed in to an account that can read $repo's releases: https://cli.github.com"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "$(uname -s) is not a system this script installs on: Windows has install.ps1" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "$(uname -m) is a processor no package is built for" ;;
esac

if [ "$tag" = latest ]; then
  tag=$(gh release list -R "$repo" --limit 1 --json tagName --jq '.[0].tagName')
  [ -n "$tag" ] || fail "$repo has no releases this account can see"
fi
archive="haas-helper_${tag}_${os}_${arch}.tar.gz"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
echo "Downloading $archive from $repo $tag"
gh release download "$tag" -R "$repo" -D "$work" -p "$archive" -p SHA256SUMS -p SHA256SUMS.sig -p SHA256SUMS.pem \
  || fail "the release $tag has no package for $os $arch, or this account cannot read it"

# The checksum of the one package, from the list the release signs.
(cd "$work" && grep " $archive\$" SHA256SUMS > wanted.sum) || fail "$archive is not in the release's checksums"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$work" && sha256sum -c wanted.sum >/dev/null) || fail "the package does not match its checksum: do not use it"
else
  (cd "$work" && shasum -a 256 -c wanted.sum >/dev/null) || fail "the package does not match its checksum: do not use it"
fi
echo "The package matches the release's checksum."

if command -v cosign >/dev/null 2>&1; then
  cosign verify-blob "$work/SHA256SUMS" --signature "$work/SHA256SUMS.sig" --certificate "$work/SHA256SUMS.pem" \
    --certificate-identity "https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com >/dev/null 2>&1 \
    || fail "the checksums' signature does not hold: do not use this package"
  echo "The checksums are signed by $repo's release workflow."
elif [ "${HAAS_REQUIRE_COSIGN:-}" = 1 ]; then
  fail "cosign is not installed, and HAAS_REQUIRE_COSIGN=1 says not to go on without it"
else
  echo "cosign is not installed, so only the checksum was checked, not who made it."
fi

tar -xzf "$work/$archive" -C "$work" haas-helper
mkdir -p "$dir"
# Put beside its place and move into it, so a program that is running is never half replaced.
install -m 0755 "$work/haas-helper" "$dir/.haas-helper.new"
mv -f "$dir/.haas-helper.new" "$dir/haas-helper"
echo "Installed $("$dir/haas-helper" version) in $dir"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "$dir is not on your PATH: add it, or run $dir/haas-helper" ;;
esac
echo "Next: haas-helper login -server https://<your organisation's server>"
