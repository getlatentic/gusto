#!/bin/sh
# Joins the per-system records the desktop builds leave into the release's desktop-manifest.json: the version, and for each system the
# file its update is and that file's signature. publish.sh turns it into the latest.json of one server.
#
#   deploy/desktop/manifest.sh v0.0.1-alpha.6 dist
set -eu
tag=${1:?the release, such as v0.0.1-alpha.6}
dir=${2:-dist}
jq -s --arg version "${tag#v}" --arg date "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg notes "$tag" '{
  version: $version, notes: $notes, pub_date: $date,
  platforms: (map({key: .platform, value: {file, signature}}) | from_entries)
}' "$dir"/desktop-*.json > "$dir/desktop-manifest.json.new"
rm -f "$dir"/desktop-*-*.json
mv "$dir/desktop-manifest.json.new" "$dir/desktop-manifest.json"
