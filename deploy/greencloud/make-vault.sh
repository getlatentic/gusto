#!/usr/bin/env bash
# Makes greencloud.vault.yml from what production on AWS holds, encrypted, without printing a secret: /gusto/cloud-config and every
# parameter the host reads. Staging takes production's settings and secrets, at the name STAGING_DOMAIN (gusto-staging. in
# production's zone, one level deep so Cloudflare's certificate covers it, unless it is set), with no shared apps. The tunnel's
# token, from the tunnel made in Cloudflare, is typed at a hidden prompt; none leaves the host's own web ports in use. Run it with an AWS profile that may read /gusto/*, and VAULT_PASSWORD_FILE naming the vault password
# (the one the other apps on the host share).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
out=$here/greencloud.vault.yml
: "${VAULT_PASSWORD_FILE:?the file holding the vault password}" "${PUBLIC_IP:?the address of the host}"
names=(/gusto/cloudflare-dns-token /gusto/cloudflare-mail-token /gusto/model-key /gusto/deepseek-key /gusto/meta-key
  /gusto/turnstile-secret /gusto/lead-hook-url /gusto/lead-hook-secret /gusto/google-client-id /gusto/google-secret
  /gusto/stripe-key /gusto/stripe-price /gusto/stripe-webhook-secret)
read -r -s -p "The Cloudflare tunnel's token (none for no tunnel): " tunnel_token; echo
plain=$(mktemp)
trap 'rm -f "$plain"' EXIT
chmod 600 "$plain"
# Parameter Store answers at most ten names a request, so they are asked for in tens and put together.
names+=(/gusto/cloud-config)
for ((i = 0; i < ${#names[@]}; i += 10)); do
  aws ssm get-parameters --with-decryption --names "${names[@]:i:10}" --query Parameters --output json
done | python3 -c '
import json, sys
text, at, found, decoder = sys.stdin.read(), 0, [], json.JSONDecoder()
while text[at:].strip():
    part, at = decoder.raw_decode(text, len(text) - len(text[at:].lstrip()))
    found += part
print(json.dumps({"Parameters": found}))' |
  PUBLIC_IP=$PUBLIC_IP STAGING_DOMAIN=${STAGING_DOMAIN:-} TUNNEL_TOKEN=$tunnel_token python3 -c '
import json, os, sys
got = {p["Name"]: p["Value"] for p in json.load(sys.stdin)["Parameters"]}
config = json.loads(got.pop("/gusto/cloud-config"))
json.dump({
    "ansible_host": os.environ["PUBLIC_IP"], "gusto_public_ip": os.environ["PUBLIC_IP"],
    "gusto_cloud_config": config, "gusto_params": got, "gusto_tunnel_token": os.environ["TUNNEL_TOKEN"],
    "gusto_staging_config": {"cloud_domain": os.environ["STAGING_DOMAIN"] or "gusto-staging." + config["cloud_zone"],
                             "cloud_apps_domain": "", "cloud_apps_zone": ""},
}, sys.stdout, indent=2)
print(sorted(got), file=sys.stderr)' > "$plain"
unset tunnel_token
ansible-vault encrypt --vault-password-file "$VAULT_PASSWORD_FILE" --output "$out" "$plain"
echo "wrote $out, encrypted"
