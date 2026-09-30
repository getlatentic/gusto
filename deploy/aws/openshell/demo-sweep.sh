#!/usr/bin/env bash
# Terminates any instance tagged project=haas-poc that has been up longer than MAX_HOURS
# (six by default). It is the backstop for a host whose own timer did not remove it, and it
# runs from somewhere that is not the host. It prints what it terminated, and says so when
# there was nothing. HAAS_NOW (an ISO time) moves the clock, which is how a test runs it.
set -euo pipefail
export AWS_REGION="${AWS_REGION:-us-east-1}" AWS_DEFAULT_REGION="${AWS_REGION:-us-east-1}"
max_hours="${MAX_HOURS:-6}"
now="${HAAS_NOW:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

listed="$(aws ec2 describe-instances \
  --filters Name=tag:project,Values=haas-poc Name=instance-state-name,Values=pending,running \
  --query 'Reservations[].Instances[].[InstanceId,LaunchTime]' --output text)"
old="$(python3 -c '
import sys, datetime
def when(text):
    return datetime.datetime.fromisoformat(text.replace("Z", "+00:00"))
now, limit = when(sys.argv[1]), float(sys.argv[2]) * 3600
for line in sys.stdin.read().splitlines():
    fields = line.split()
    if len(fields) == 2 and (now - when(fields[1])).total_seconds() > limit:
        print(fields[0])
' "$now" "$max_hours" <<<"$listed")"
if [ -z "$old" ]; then
  echo "nothing tagged haas-poc has been up longer than $max_hours hours"
  exit 0
fi
for id in $old; do
  aws ec2 terminate-instances --instance-ids "$id" >/dev/null
  echo "terminated $id, which had been up longer than $max_hours hours"
done
