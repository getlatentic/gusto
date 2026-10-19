#!/usr/bin/env bash
# Stores where leads are sent when a demo starts and ends: the address of the Apps Script web app of the Google Sheet,
# and the secret that the script checks. Both are read at hidden prompts, so they are on no command line and in no
# history, and are kept as SecureStrings that the front door's host reads with its role. The secret is the script's
# property SECRET, which the script's setup function makes, and which is read in the script's Project Settings.
source "$(dirname "$0")/env.sh"
read -r -s -p "The web app address (https://script.google.com/macros/s/.../exec): " address
echo
case "$address" in https://script.google.com/macros/s/*/exec) ;; *) echo "that is not an Apps Script web app address" >&2; exit 2;; esac
read -r -s -p "The secret (the script's property SECRET): " secret
echo
[ -n "$secret" ] || { echo "nothing was typed" >&2; exit 2; }
# The AWS command fetches a value that begins with https:// as a web address, so each value is given as a file, which it
# reads as it is. The files are readable by this user only and are gone when this ends.
umask 077
held="$(mktemp -d)"
trap 'rm -rf "$held"' EXIT
printf '%s' "$address" > "$held/url"
printf '%s' "$secret" > "$held/secret"
unset address secret
aws ssm put-parameter --name /gusto/poc/lead-hook-url --type SecureString --overwrite --value "file://$held/url" --query Version --output text >/dev/null
aws ssm put-parameter --name /gusto/poc/lead-hook-secret --type SecureString --overwrite --value "file://$held/secret" --query Version --output text >/dev/null
echo "stored /gusto/poc/lead-hook-url and /gusto/poc/lead-hook-secret in $AWS_REGION"
