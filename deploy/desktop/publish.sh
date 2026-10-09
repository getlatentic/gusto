#!/bin/sh
# Puts a release's desktop app where a server hands it out. The app updates itself from /desktop/latest.json on the server it is
# connected to, and that file names each system's update by an address on that server, so it is made here for one server.
#
#   deploy/desktop/publish.sh v0.0.1-alpha.6 /srv/haas/desktop https://haas.example.com
#
# Then start the server with HAAS_DESKTOP=/srv/haas/desktop. Needs the GitHub command line signed in, for a private repository, and jq.
set -eu
tag=${1:?the release, such as v0.0.1-alpha.6}
dir=${2:?the folder the server hands out, which HAAS_DESKTOP names}
base=${3:?the address of the server, such as https://haas.example.com}
repo=${HAAS_REPO:-getlatentic/haas}

mkdir -p "$dir"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
gh release download "$tag" -R "$repo" -D "$work" -p desktop-manifest.json
for file in $(jq -r '.platforms[].file' "$work/desktop-manifest.json"); do
  gh release download "$tag" -R "$repo" -D "$dir" -p "$file" --clobber
done

base=${base%/}
jq --arg base "$base/desktop" '{
  version, notes, pub_date,
  platforms: (.platforms | map_values({signature, url: ($base + "/" + .file)}))
}' "$work/desktop-manifest.json" > "$dir/latest.json.new"
mv "$dir/latest.json.new" "$dir/latest.json"
echo "$dir/latest.json: $(jq -r .version "$dir/latest.json") for $(jq -r '.platforms | keys | join(", ")' "$dir/latest.json")"
