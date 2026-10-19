#!/usr/bin/env bash
# Stores a model's key in Parameter Store as a SecureString, which the test
# instance's role may read (main.tf) and nothing else may read without your AWS
# profile. The standard tier and the AWS-managed key cost nothing. The key is
# typed at a hidden prompt, or piped in, and never in your shell history or in
# a process's arguments. It outlives ./down.sh, so it
# is stored once; delete it with:  aws ssm delete-parameter --name /gusto/poc/model-key
source "$(dirname "$0")/env.sh"
name="${GUSTO_MODEL_PARAMETER:-/gusto/poc/model-key}"
case "$name" in /gusto/poc/*) ;; *) echo "the instance's role reads only /gusto/poc/*: $name" >&2; exit 2 ;; esac
read -rsp "key for $name: " key; echo
[ -n "$key" ] || { echo "no key given" >&2; exit 2; }
# A file only you can read, removed on exit: the CLI reads its input twice, so a pipe
# would arrive empty, and an argument would show in the process list.
input="$(umask 077; mktemp)"; trap 'rm -f "$input"' EXIT
printf '%s' "$key" | python3 -c '
import json,sys
print(json.dumps({"Name":sys.argv[1],"Type":"SecureString","Overwrite":True,"Value":sys.stdin.read()}))' "$name" > "$input"
aws ssm put-parameter --cli-input-json "file://$input" --output text --query Version
unset key
echo "stored $name in $AWS_REGION"
