#!/usr/bin/env bash
# Stores what a demo host needs to know that is not secret, from demo.env, where the host reads
# it: the name, the zone, the mail sender and so on. The tokens are stored with put-cloudflare.sh.
# It is stored again whenever demo.env changes.
source "$(dirname "$0")/env.sh"
[ -f demo.env ] || { echo "no demo.env: copy demo.env.example and fill it in" >&2; exit 2; }
set -a; source demo.env; set +a
for need in DEMO_NAME DEMO_ZONE DEMO_ACCOUNT_ID DEMO_MAIL_FROM DEMO_CERT_EMAIL; do
  [ -n "${!need:-}" ] || { echo "$need is not set in demo.env" >&2; exit 2; }
done
config="$(python3 - <<'PY'
import json, os, urllib.parse
mail = "cloudflare://%s?from=%s" % (os.environ["DEMO_ACCOUNT_ID"], urllib.parse.quote(os.environ["DEMO_MAIL_FROM"]))
print(json.dumps({
    "demo_name": os.environ["DEMO_NAME"], "demo_zone": os.environ["DEMO_ZONE"], "demo_mail": mail,
    "demo_cert_email": os.environ["DEMO_CERT_EMAIL"], "demo_free_mail": os.environ.get("DEMO_FREE_MAIL", "allow"),
    "demo_models": os.environ.get("DEMO_MODELS", "platform/qwen.qwen3-vl-235b-a22b-instruct"),
    "demo_computer_memory": os.environ.get("DEMO_COMPUTER_MEMORY", "3Gi"),
    "demo_turnstile_site": os.environ.get("DEMO_TURNSTILE_SITE", ""),
    "demo_visitor_minutes": int(os.environ.get("DEMO_VISITOR_MINUTES", "0")),
    "demo_visitor_max": int(os.environ.get("DEMO_VISITOR_MAX", "5")),
    "demo_book_url": os.environ.get("DEMO_BOOK_URL", ""),
    "demo_video_url": os.environ.get("DEMO_VIDEO_URL", ""),
}))
PY
)"
aws ssm put-parameter --name /haas/poc/demo-config --type String --overwrite --value "$config" --query Version --output text
echo "stored /haas/poc/demo-config in $AWS_REGION"
