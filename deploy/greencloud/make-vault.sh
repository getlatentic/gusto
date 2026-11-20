#!/usr/bin/env bash
# Makes greencloud.vault.yml from what production on AWS holds, encrypted, without printing a secret: /gusto/cloud-config and every
# parameter the host reads. Run it with an AWS profile that may read /gusto/*, and VAULT_PASSWORD_FILE naming the vault password
# (the one the other apps on the host share).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
out=$here/greencloud.vault.yml
: "${VAULT_PASSWORD_FILE:?the file holding the vault password}" "${PUBLIC_IP:?the address of the host}"
names=(/gusto/cloudflare-dns-token /gusto/cloudflare-mail-token /gusto/model-key /gusto/deepseek-key /gusto/meta-key
  /gusto/turnstile-secret /gusto/lead-hook-url /gusto/lead-hook-secret /gusto/google-client-id /gusto/google-secret
  /gusto/stripe-key /gusto/stripe-price /gusto/stripe-webhook-secret)
plain=$(mktemp)
trap 'rm -f "$plain"' EXIT
chmod 600 "$plain"
aws ssm get-parameters --with-decryption --names "${names[@]}" /gusto/cloud-config --output json |
  PUBLIC_IP=$PUBLIC_IP python3 -c '
import json, os, sys
got = {p["Name"]: p["Value"] for p in json.load(sys.stdin)["Parameters"]}
config = json.loads(got.pop("/gusto/cloud-config"))
json.dump({
    "ansible_host": os.environ["PUBLIC_IP"], "gusto_public_ip": os.environ["PUBLIC_IP"],
    "gusto_cloud_config": config, "gusto_params": got,
}, sys.stdout, indent=2)
print(sorted(got), file=sys.stderr)' > "$plain"
ansible-vault encrypt --vault-password-file "$VAULT_PASSWORD_FILE" --output "$out" "$plain"
echo "wrote $out, encrypted"
