#!/usr/bin/env bash
# Stores the OAuth client at Google that people sign in to gusto cloud with. Make it in the Google Cloud console as a web
# application whose authorized redirect address is https://<cloud domain>/signin/google/back. The secret is read at a hidden prompt,
# so it is on no command line and in no history. Both are kept as SecureStrings that a front door reads with its role at boot.
source "$(dirname "$0")/env.sh"
read -r -p "The Google client id: " id
read -r -s -p "The Google client secret: " secret
echo
[ -n "$id" ] && [ -n "$secret" ] || { echo "both the id and the secret are needed" >&2; exit 2; }
aws ssm put-parameter --name /gusto/google-client-id --type SecureString --overwrite --value "$id" --query Version --output text >/dev/null
aws ssm put-parameter --name /gusto/google-secret --type SecureString --overwrite --value "$secret" --query Version --output text >/dev/null
unset secret
echo "stored /gusto/google-client-id and /gusto/google-secret in $AWS_REGION; a front door reads them at its next boot"
