#!/usr/bin/env bash
# Measures what the nested VM costs: bench/work.py on the host, then inside a
# Firecracker sandbox, and how long sandboxes take to start. Over SSM.
source "$(dirname "$0")/env.sh"
group="$(terraform output -raw autoscaling_group)"
instance="$(aws autoscaling describe-auto-scaling-groups --auto-scaling-group-names "$group" \
  --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`] | [0].InstanceId' --output text)"
[ "$instance" != "None" ] || { echo "no running instance in $group" >&2; exit 1; }
work="$(base64 < bench/work.py | tr -d '\n')"
bench="$(base64 < bench/bench.mjs | tr -d '\n')"
parameters="$(python3 - "$work" "$bench" <<'PY'
import json, sys
work, bench = sys.argv[1], sys.argv[2]
print(json.dumps({"commands": [
    f"echo {work} | base64 -d > /tmp/work.py",
    f"echo {bench} | base64 -d > /tmp/bench.mjs",
    "echo host: $(python3 /tmp/work.py)",
    "cd /opt/e2b && docker compose --profile test run --rm --no-deps "
    "-v /tmp/bench.mjs:/app/bench.mjs:ro -v /tmp/work.py:/tmp/work.py:ro --entrypoint /bin/sh smoke "
    "-c 'E2B_API_KEY=$(cat /run/e2b/team-api-key) node /app/bench.mjs' 2>&1 | tail -3",
]}))
PY
)"
command="$(aws ssm send-command --instance-ids "$instance" --document-name AWS-RunShellScript \
  --comment "haas POC benchmark" --timeout-seconds 1200 --parameters "$parameters" \
  --query Command.CommandId --output text)"
printf 'benchmark on %s ' "$instance"
while :; do
  status="$(aws ssm get-command-invocation --command-id "$command" --instance-id "$instance" --query Status --output text 2>/dev/null || echo Pending)"
  case "$status" in Pending|InProgress|Delayed) printf '.'; sleep 5 ;; *) break ;; esac
done
echo " $status"
aws ssm get-command-invocation --command-id "$command" --instance-id "$instance" \
  --query '[StandardOutputContent,StandardErrorContent]' --output text
[ "$status" = Success ]
