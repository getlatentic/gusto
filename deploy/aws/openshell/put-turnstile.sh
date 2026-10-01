#!/usr/bin/env bash
# Stores the secret of the bot check on the demo form, which Cloudflare shows once when the Turnstile widget is made. It is
# read at a hidden prompt, so it is on no command line and in no history, and is kept as a SecureString that a host reads
# with its role. The site key, which is public, goes in demo.env as DEMO_TURNSTILE_SITE and is stored by put-demo-config.sh.
source "$(dirname "$0")/env.sh"
name=/haas/poc/turnstile-secret
read -r -s -p "The Turnstile secret key: " secret
echo
[ -n "$secret" ] || { echo "nothing was typed" >&2; exit 2; }
aws ssm put-parameter --name "$name" --type SecureString --overwrite --value "$secret" --query Version --output text >/dev/null
unset secret
echo "stored $name in $AWS_REGION"
