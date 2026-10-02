#!/usr/bin/env bash
# Checks, from inside a running demo computer, that an agent cannot reach the operator's network: the machine's AWS metadata
# address (whose role reads the stored secrets), the front door, the shared file system, the resolver, the machine itself and the
# internet. It passes when only the model proxy is open. It needs a demo with an agent running, since a sandbox is made for one:
# start a demo, start an agent in it, and run this. Exits 0 on a pass.
set -uo pipefail
: "${AWS_PROFILE:=haas-poc}"
export AWS_PROFILE
here="$(cd "$(dirname "$0")" && pwd)"
cell="$(aws ec2 describe-instances --filters Name=tag:scaler,Values=on Name=instance-state-name,Values=running --query 'Reservations[].Instances[].InstanceId' --output text | awk '{print $1}')"
[ -n "$cell" ] || { echo "no demo machine is running: start a demo, and an agent in it" >&2; exit 2; }
front="$(aws ec2 describe-instances --filters Name=tag:project,Values=haas-live Name=instance-state-name,Values=running --query 'Reservations[].Instances[?!(Tags[?Key==`scaler`])].PrivateIpAddress' --output text)"
script="$(base64 < "$here/e2e/isolation-on-host.sh" | tr -d '\n')"
id="$(aws ssm send-command --instance-ids "$cell" --document-name AWS-RunShellScript \
  --parameters "commands=[\"echo $script | base64 -d > /tmp/isolation.sh && sudo FRONT_DOOR=$front bash /tmp/isolation.sh 2>&1\"]" --query Command.CommandId --output text)"
for _ in $(seq 1 30); do
  status="$(aws ssm get-command-invocation --command-id "$id" --instance-id "$cell" --query Status --output text 2>/dev/null || true)"
  case "$status" in Success|Failed|Cancelled|TimedOut) break ;; esac
  sleep 4
done
aws ssm get-command-invocation --command-id "$id" --instance-id "$cell" --query StandardOutputContent --output text
[ "$status" = Success ]
