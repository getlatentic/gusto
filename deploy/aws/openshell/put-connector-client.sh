#!/usr/bin/env bash
# Stores the OAuth client the operator registered with an app that needs one, such as GitHub, for every organization's server
# to sign people in to it with. The id is an argument; the secret is read at a hidden prompt, so it is on no command line and
# in no history. The clients are kept together as one SecureString that the hosts read with their role, and an organization's
# server reads it when it starts, so the next deploy brings it in.
#   deploy/aws/openshell/put-connector-client.sh github Ov23liAbCdEf
source "$(dirname "$0")/env.sh"
name=/gusto/poc/connector-clients
connector=${1:?the connector, such as github}
id=${2:?the client id the app gave}
read -r -s -p "The client secret for $connector: " secret
echo
[ -n "$secret" ] || { echo "nothing was typed" >&2; exit 2; }
held=$(aws ssm get-parameter --name "$name" --with-decryption --query Parameter.Value --output text 2>/dev/null || echo '{}')
# The secret goes to python on its input, not as an argument, and the merged value to aws in a file only this user can read.
value=$(umask 077; printf '%s' "$secret" | CONNECTOR="$connector" ID="$id" HELD="$held" /usr/bin/python3 -I -c '
import json, os, sys
clients = json.loads(os.environ["HELD"])
clients[os.environ["CONNECTOR"]] = {"id": os.environ["ID"], "secret": sys.stdin.read()}
print(json.dumps(clients))')
unset secret
file=$(mktemp)
trap 'rm -f "$file"' EXIT
printf '%s' "$value" > "$file"
unset value
aws ssm put-parameter --name "$name" --type SecureString --overwrite --value "file://$file" --query Version --output text >/dev/null
echo "stored the $connector client in $name in $AWS_REGION"
