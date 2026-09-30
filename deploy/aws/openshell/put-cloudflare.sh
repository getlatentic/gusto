#!/usr/bin/env bash
# Stores the two Cloudflare API tokens the demo host reads from Parameter Store, each typed
# at a hidden prompt (see put-key.sh):
#   dns   edits DNS records in the zone the demo name is under (Zone > DNS > Edit, that zone only).
#         It points the name at the host and answers Let's Encrypt's DNS challenge.
#   mail  sends mail (Account > Email Sending > Edit).
# They outlive ./down.sh, so they are stored once.
cd "$(dirname "$0")"
for which in dns mail; do
  echo "== the $which token"
  HAAS_MODEL_PARAMETER="/haas/poc/cloudflare-$which-token" ./put-key.sh
done
