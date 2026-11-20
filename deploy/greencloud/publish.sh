#!/usr/bin/env bash
# Copies a release from the bucket on AWS to the store a host off AWS reads, after CI has put it in the bucket: the bundle and its
# sum, the runner image when the store lacks it, and the front door's media. The release is made the newest last, so a host never
# sees a newest release whose files are not there. CI runs it with the store's key in STORE_KEY_ID and STORE_SECRET.
set -euo pipefail
: "${BUCKET:?the bucket on AWS}" "${STORE:?the store bucket}" "${ENDPOINT:?the store endpoint}" "${RELEASE:?the release}"
: "${STORE_KEY_ID:?the store key id}" "${STORE_SECRET:?the store secret}"
store() {
  AWS_ACCESS_KEY_ID=$STORE_KEY_ID AWS_SECRET_ACCESS_KEY=$STORE_SECRET AWS_SESSION_TOKEN='' \
    aws s3 "$@" --endpoint-url "$ENDPOINT" --only-show-errors
}
stored() {
  AWS_ACCESS_KEY_ID=$STORE_KEY_ID AWS_SECRET_ACCESS_KEY=$STORE_SECRET AWS_SESSION_TOKEN='' \
    aws s3api head-object --bucket "$STORE" --key "$1" --endpoint-url "$ENDPOINT" >/dev/null 2>&1
}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

for file in bundle.tgz bundle.tgz.sha256; do
  aws s3 cp "s3://$BUCKET/releases/$RELEASE/$file" "$tmp/$file" --only-show-errors
  store cp "$tmp/$file" "s3://$STORE/releases/$RELEASE/$file"
done
key=$(tar -xzf "$tmp/bundle.tgz" -O ./image.key)
if ! stored "image/$key.docker.tar.gz"; then
  aws s3 cp "s3://$BUCKET/image/$key.docker.tar.gz" "$tmp/image.docker.tar.gz" --only-show-errors
  store cp "$tmp/image.docker.tar.gz" "s3://$STORE/image/$key.docker.tar.gz"
  echo "copied the runner image $key to the store"
fi
aws s3 sync "s3://$BUCKET/media/" "$tmp/media/" --only-show-errors
store sync "$tmp/media/" "s3://$STORE/media/" --delete
printf '%s' "$RELEASE" > "$tmp/latest"
store cp "$tmp/latest" "s3://$STORE/releases/latest"
echo "release $RELEASE is the newest in the store"
