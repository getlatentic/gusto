#!/usr/bin/env bash
# Stores where leads are sent when a demo starts and ends: the address of the Apps Script web app of the Google Sheet,
# and the secret that the script checks. Both are read at hidden prompts, so they are on no command line and in no
# history, and are kept as SecureStrings that the front door's host reads with its role. A secret left empty is made
# here and shown once, to be put in the script's properties as SECRET.
source "$(dirname "$0")/env.sh"
read -r -s -p "The web app address (https://script.google.com/macros/s/.../exec): " address
echo
case "$address" in https://script.google.com/macros/s/*/exec) ;; *) echo "that is not an Apps Script web app address" >&2; exit 2;; esac
read -r -s -p "The secret, or Enter to make one: " secret
echo
if [ -z "$secret" ]; then
  secret="$(openssl rand -hex 24)"
  echo "The secret is $secret"
  echo "Put it in the script's properties as SECRET, now, since it is not shown again."
fi
aws ssm put-parameter --name /haas/poc/lead-hook-url --type SecureString --overwrite --value "$address" --query Version --output text >/dev/null
aws ssm put-parameter --name /haas/poc/lead-hook-secret --type SecureString --overwrite --value "$secret" --query Version --output text >/dev/null
unset address secret
echo "stored /haas/poc/lead-hook-url and /haas/poc/lead-hook-secret in $AWS_REGION"
