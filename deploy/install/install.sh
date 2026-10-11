#!/bin/sh
# Installs haas and haas-helper on macOS or Linux from a release: downloads each package for this system, checks it against the
# release's checksums, and checks the checksums' signature when cosign is installed. It downloads with the GitHub command line
# when that is signed in, which a private release needs, and otherwise straight from the release's public address.
#
#   curl -fsSL https://github.com/getlatentic/haas/releases/latest/download/install.sh | sh
#   deploy/install/install.sh                                  # the newest release, an alpha included
#   deploy/install/install.sh v0.0.1-alpha.5
#   HAAS_INSTALL_DIR=/usr/local/bin deploy/install/install.sh  # where to put them; ~/.local/bin by default
#   HAAS_PROGRAMS=haas-helper deploy/install/install.sh         # which to install; both by default
#   HAAS_REQUIRE_COSIGN=1 deploy/install/install.sh             # refuse to go on without checking the signature
#   HAAS_NO_START=1 deploy/install/install.sh                   # install, and do not start haas for this person
#   HAAS_RELEASE_URL=http://10.0.2.2:8000 deploy/install/install.sh v0.0.0-test   # a release's files from another address
#
# Once haas is installed it is started for the person running this, as a service that starts at login, and opened in their
# browser, signed in: nothing more is needed to begin.
set -eu

repo=${HAAS_REPO:-getlatentic/haas}
tag=${1:-latest}
dir=${HAAS_INSTALL_DIR:-${HAAS_HELPER_DIR:-$HOME/.local/bin}}
programs=${HAAS_PROGRAMS:-haas haas-helper}

fail() { echo "install: $*" >&2; exit 1; }

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

# The GitHub command line reads a private release; a public one is read with curl and no account. HAAS_RELEASE_URL is a folder
# with a release's files in it, such as a test build served to a machine that cannot reach GitHub.
if [ -n "${HAAS_RELEASE_URL:-}" ]; then via=url
elif command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then via=gh; else via=curl; fi

newest() {
  if [ "$via" = gh ]; then
    gh release list -R "$repo" --limit 1 --json tagName --jq '.[0].tagName'
  else
    curl -fsSL "https://api.github.com/repos/$repo/releases?per_page=1" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1
  fi
}

fetch() {
  if [ "$via" = gh ]; then
    gh release download "$tag" -R "$repo" -D "$work" -p "$1"
  elif [ "$via" = url ]; then
    curl -fsSL -o "$work/$1" "${HAAS_RELEASE_URL%/}/$1"
  else
    curl -fsSL -o "$work/$1" "https://github.com/$repo/releases/download/$tag/$1"
  fi
}

if [ "$tag" = latest ]; then
  [ "$via" != url ] || fail "HAAS_RELEASE_URL needs the release's tag, as the first argument"
  tag=$(newest) || true
  [ -n "$tag" ] || fail "$repo has no release this machine can see: a private one needs the GitHub command line, signed in (https://cli.github.com)"
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fetch SHA256SUMS || fail "the release $tag cannot be read from here"

if command -v cosign >/dev/null 2>&1; then
  fetch SHA256SUMS.sigstore.json || fail "the release $tag has no signature for its checksums"
  cosign verify-blob "$work/SHA256SUMS" --bundle "$work/SHA256SUMS.sigstore.json" \
    --certificate-identity "https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com >/dev/null 2>&1 \
    || fail "the checksums' signature does not hold: do not use these packages"
  echo "The checksums are signed by $repo's release workflow."
elif [ "${HAAS_REQUIRE_COSIGN:-}" = 1 ]; then
  fail "cosign is not installed, and HAAS_REQUIRE_COSIGN=1 says not to go on without it"
else
  echo "cosign is not installed, so the packages are checked against the checksums, and not who made them."
fi

matches() {
  (cd "$work" && grep " $1\$" SHA256SUMS > "$1.sum") || fail "$1 is not in the release's checksums"
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$work" && sha256sum -c "$1.sum" >/dev/null) || fail "$1 does not match its checksum: do not use it"
  else
    (cd "$work" && shasum -a 256 -c "$1.sum" >/dev/null) || fail "$1 does not match its checksum: do not use it"
  fi
}

mkdir -p "$dir"
for program in $programs; do
  archive="${program}_${tag}_${os}_${arch}.tar.gz"
  echo "Downloading $archive from $repo $tag"
  fetch "$archive" || fail "the release $tag has no $program for $os $arch"
  matches "$archive"
  tar -xzf "$work/$archive" -C "$work" "$program"
  # Put beside its place and move into it, so a program that is running is never half replaced.
  install -m 0755 "$work/$program" "$dir/.$program.new"
  mv -f "$dir/.$program.new" "$dir/$program"
  echo "Installed $("$dir/$program" version) in $dir"
done
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "$dir is not on your PATH: add it, or run the programs from there" ;;
esac
case " $programs " in
  *" haas "*) [ "${HAAS_NO_START:-}" = 1 ] || "$dir/haas" local install ;;
esac
