#!/usr/bin/env bash
# Stores what a host needs to know that is not secret, from cloud.env, where the host reads
# it: the name, the zone, the mail sender and so on. The tokens are stored with put-cloudflare.sh.
# It is stored again whenever cloud.env changes.
source "$(dirname "$0")/env.sh"
[ -f cloud.env ] || { echo "no cloud.env: copy cloud.env.example and fill it in" >&2; exit 2; }
set -a; source cloud.env; set +a
for need in GUSTO_CLOUD_DOMAIN GUSTO_CLOUD_ZONE GUSTO_CLOUDFLARE_ACCOUNT_ID GUSTO_MAIL_FROM GUSTO_CERT_EMAIL; do
  [ -n "${!need:-}" ] || { echo "$need is not set in cloud.env" >&2; exit 2; }
done
config="$(python3 - <<'PY'
import json, os, urllib.parse
mail = "cloudflare://%s?from=%s" % (os.environ["GUSTO_CLOUDFLARE_ACCOUNT_ID"], urllib.parse.quote(os.environ["GUSTO_MAIL_FROM"]))
print(json.dumps({
    "cloud_domain": os.environ["GUSTO_CLOUD_DOMAIN"], "cloud_zone": os.environ["GUSTO_CLOUD_ZONE"], "cloud_mail": mail,
    "cloud_apps_domain": os.environ.get("GUSTO_CLOUD_APPS_DOMAIN", ""), "cloud_apps_zone": os.environ.get("GUSTO_CLOUD_APPS_ZONE", ""),
    "cloud_cert_email": os.environ["GUSTO_CERT_EMAIL"], "cloud_free_mail": os.environ.get("GUSTO_CLOUD_FREE_MAIL", "allow"),
    "cloud_models": os.environ.get("GUSTO_MODELS", "gemma/google.gemma-4-26b-a4b,gemma/google.gemma-4-31b,deepseek/deepseek-flash,meta/muse-spark-1.3"),
    "cloud_computer_memory": os.environ.get("GUSTO_COMPUTER_MEMORY", "3Gi"),
    "cloud_turnstile_site": os.environ.get("GUSTO_CLOUD_TURNSTILE_SITE", ""),
    "cloud_demo_minutes": int(os.environ.get("GUSTO_CLOUD_DEMO_MINUTES", "0")),
    "cloud_book_url": os.environ.get("GUSTO_CLOUD_BOOK_URL", ""),
    "cloud_demo_notify": os.environ.get("GUSTO_CLOUD_DEMO_NOTIFY", ""),
    "cloud_demo_staff": os.environ.get("GUSTO_CLOUD_DEMO_STAFF", ""),
}))
PY
)"
aws ssm put-parameter --name /gusto/cloud-config --type String --overwrite --value "$config" --query Version --output text
echo "stored /gusto/cloud-config in $AWS_REGION"
