#!/bin/sh
# Builds the desktop app for one Rust target and puts what a release carries in one folder: the installer, the file the app updates
# itself from with its signature, and a small record of which system that is. Run from the root of the repository, with the update
# signing key in TAURI_SIGNING_PRIVATE_KEY.
#
#   deploy/desktop/build.sh v0.0.1-alpha.6 aarch64-apple-darwin dist
set -eu
tag=${1:?the release, such as v0.0.1-alpha.6}
triple=${2:?the Rust target, such as aarch64-apple-darwin}
out=${3:-dist}
version=${tag#v}

case "$triple" in
  aarch64-apple-darwin)      platform=darwin-aarch64  bundles=app,dmg ;;
  x86_64-apple-darwin)       platform=darwin-x86_64   bundles=app,dmg ;;
  x86_64-pc-windows-msvc)    platform=windows-x86_64  bundles=nsis ;;
  x86_64-unknown-linux-gnu)  platform=linux-x86_64    bundles=appimage,deb ;;
  *) echo "the app is not built for $triple" >&2; exit 2 ;;
esac

root=$(git rev-parse --show-toplevel)
case "$out" in /*) ;; *) out="$root/$out" ;; esac
GUSTO_VERSION=$tag "$root/desktop/prepare.sh" "$triple" >/dev/null
cd "$root/desktop/src-tauri"
cargo tauri build --target "$triple" --bundles "$bundles" \
  --config "{\"version\":\"$version\",\"bundle\":{\"createUpdaterArtifacts\":true}}"

bundle="target/$triple/release/bundle"
mkdir -p "$out"
name="latentic-agent-cloud_${tag}_${platform}"

keep() { # keep <file> <suffix>
  cp "$1" "$out/$name$2"
}

update=""
case "$platform" in
  darwin-*)
    update=$(ls "$bundle"/macos/*.app.tar.gz); keep "$update" .app.tar.gz; keep "$update.sig" .app.tar.gz.sig
    update=$name.app.tar.gz
    keep "$(ls "$bundle"/dmg/*.dmg)" .dmg ;;
  windows-*)
    update=$(ls "$bundle"/nsis/*-setup.exe); keep "$update" -setup.exe; keep "$update.sig" -setup.exe.sig
    update=$name-setup.exe ;;
  linux-*)
    update=$(ls "$bundle"/appimage/*.AppImage); keep "$update" .AppImage; keep "$update.sig" .AppImage.sig
    update=$name.AppImage
    keep "$(ls "$bundle"/deb/*.deb)" .deb ;;
esac

# The signature is the text of the .sig file, which is what the updater's manifest carries.
printf '{"platform":"%s","file":"%s","signature":"%s"}\n' "$platform" "$update" "$(tr -d '\n' < "$out/$update.sig")" \
  > "$out/desktop-$platform.json"
echo "$out/$update"
