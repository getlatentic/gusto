#!/usr/bin/env bash
# Opens the operator's page of the live public cloud on this machine, at http://localhost:8090.
#
# The page runs on the host of gusto cloud and listens on that host's loopback only, so it is
# not reachable from the internet, and the web server of gusto cloud does not serve it. This opens a
# tunnel to it with AWS Systems Manager, which is allowed by the same AWS identity that manages the
# platform. It needs the Session Manager plugin: brew install --cask session-manager-plugin
#
# PORT=9000 ./owner.sh uses another port here.
set -euo pipefail
: "${AWS_PROFILE:=haas-poc}"
export AWS_PROFILE
port="${PORT:-8090}"
command -v session-manager-plugin >/dev/null || { echo "owner.sh: install the Session Manager plugin first: brew install --cask session-manager-plugin" >&2; exit 1; }
# gusto cloud is the live host that is not one of the hosts it started, which are tagged scaler=on.
front="$(aws ec2 describe-instances \
  --filters Name=tag:project,Values=haas-live Name=instance-state-name,Values=running \
  --query 'Reservations[].Instances[?!(Tags[?Key==`scaler`])].InstanceId' --output text)"
[ -n "$front" ] && [ "$(wc -w <<<"$front")" -eq 1 ] || { echo "owner.sh: expected one running host of gusto cloud, found: ${front:-none}" >&2; exit 1; }
echo "the operator's page is at http://localhost:$port while this runs; Ctrl-C closes it"
exec aws ssm start-session --target "$front" --document-name AWS-StartPortForwardingSession \
  --parameters "portNumber=8090,localPortNumber=$port"
