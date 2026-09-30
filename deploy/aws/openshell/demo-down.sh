#!/usr/bin/env bash
# Takes the demo name's DNS records away, then removes the host and checks that nothing of
# it is left. The store bucket, the tokens and the model keys stay.
cd "$(dirname "$0")"
source ./env.sh
if address="$(terraform output -raw address 2>/dev/null)" && [ -n "$address" ]; then
  ssh "${ssh_options[@]}" "ubuntu@$address" 'sudo systemctl stop haas-cloud haas-boot' || echo "the DNS records could not be removed from the host: remove them in Cloudflare" >&2
fi
./down.sh
