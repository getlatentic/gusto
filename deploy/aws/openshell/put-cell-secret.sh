#!/usr/bin/env bash
# Makes the secret the front door of a demo and its cells sign their requests with, and stores it where the
# hosts read it. It is made here, never shown, and not made again when there is one: a demo that is up
# uses the one it started with.
source "$(dirname "$0")/env.sh"
name=/gusto/poc/cell-secret
if aws ssm get-parameter --name "$name" --query Parameter.Name --output text >/dev/null 2>&1; then
  echo "$name is stored already"
  exit 0
fi
secret="$(python3 -c 'import secrets; print(secrets.token_urlsafe(48))')"
aws ssm put-parameter --name "$name" --type SecureString --value "$secret" --query Version --output text >/dev/null
echo "stored $name in $AWS_REGION"
