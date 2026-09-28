#!/usr/bin/env bash
# Runs E2B's own smoke test on the instance: a sandbox created, a port reached
# in it, paused and resumed. It runs over SSM, so no port is opened for it.
source "$(dirname "$0")/env.sh"
group="$(terraform output -raw autoscaling_group)"
instance="$(aws autoscaling describe-auto-scaling-groups --auto-scaling-group-names "$group" \
  --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`] | [0].InstanceId' --output text)"
[ "$instance" != "None" ] || { echo "no running instance in $group" >&2; exit 1; }
command="$(aws ssm send-command --instance-ids "$instance" --document-name AWS-RunShellScript \
  --comment "haas POC smoke" --timeout-seconds 900 \
  --parameters 'commands=["cd /opt/e2b && time docker compose --profile test run --rm smoke 2>&1 | tail -40","ls -l /dev/kvm","nproc; free -g | head -2"]' \
  --query Command.CommandId --output text)"
printf 'smoke on %s ' "$instance"
while :; do
  status="$(aws ssm get-command-invocation --command-id "$command" --instance-id "$instance" --query Status --output text 2>/dev/null || echo Pending)"
  case "$status" in Pending|InProgress|Delayed) printf '.'; sleep 5 ;; *) break ;; esac
done
echo " $status"
aws ssm get-command-invocation --command-id "$command" --instance-id "$instance" \
  --query '[StandardOutputContent,StandardErrorContent]' --output text
[ "$status" = Success ]
