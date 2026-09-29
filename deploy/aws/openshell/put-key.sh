#!/usr/bin/env bash
# Stores a model's key in Parameter Store as a SecureString, which the test
# instance's role may read (main.tf) and nothing else may read without your AWS
# profile. The standard tier and the AWS-managed key cost nothing. The key is
# typed at a hidden prompt and sent to AWS on standard input, so it is never in
# your shell history or in a process's arguments. It outlives ./down.sh, so it
# is stored once; delete it with:  aws ssm delete-parameter --name /haas/poc/model-key
source "$(dirname "$0")/env.sh"
name="${HAAS_MODEL_PARAMETER:-/haas/poc/model-key}"
case "$name" in /haas/poc/*) ;; *) echo "the instance's role reads only /haas/poc/*: $name" >&2; exit 2 ;; esac
read -rsp "key for $name: " key; echo
[ -n "$key" ] || { echo "no key given" >&2; exit 2; }
printf '%s' "$key" | python3 -c '
import json,sys
print(json.dumps({"Name":sys.argv[1],"Type":"SecureString","Overwrite":True,"Value":sys.stdin.read()}))' "$name" \
  | aws ssm put-parameter --cli-input-json file:///dev/stdin --output text --query Version
unset key
echo "stored $name in $AWS_REGION"
