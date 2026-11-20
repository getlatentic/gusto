#!/usr/bin/env bash
# Moves production's data from the front door on AWS to the host off AWS, once: the accounts database, the task index and the
# landing page's media from the front door's disk, and the organizations, demos and records from the shared file system, without
# the copies of the program, which the new host makes for itself. gusto cloud is stopped on both while it moves, and started on the new host after.
# Run it with the AWS profile of production, after site.yml has made the host, and before the name is pointed at it.
set -euo pipefail
: "${FRONT_DOOR:?the instance id of the front door}" "${HOST:?root@ the host off AWS}" "${BUCKET:?the bucket on AWS}"
key=move/$(date -u +%Y%m%dT%H%M%SZ).tgz
here=$(mktemp -d)
trap 'rm -rf "$here"' EXIT

on_aws() {
  local id state
  id=$(aws ssm send-command --instance-ids "$FRONT_DOOR" --document-name AWS-RunShellScript \
    --parameters "commands=[\"export HOME=/root PATH=\$PATH:/snap/bin; $1\"]" --query Command.CommandId --output text)
  for _ in $(seq 1 60); do
    state=$(aws ssm get-command-invocation --command-id "$id" --instance-id "$FRONT_DOOR" --query Status --output text 2>/dev/null || echo Pending)
    case "$state" in
    Success) return 0 ;;
    Failed | Cancelled | TimedOut) aws ssm get-command-invocation --command-id "$id" --instance-id "$FRONT_DOOR" --query StandardErrorContent --output text >&2; return 1 ;;
    esac
    sleep 5
  done
  echo "the command on $FRONT_DOOR did not finish" >&2
  return 1
}

echo "stopping gusto cloud on $FRONT_DOOR and packing its data"
on_aws "systemctl stop gusto-owner gusto-cloud; if ! { tar -czf /tmp/move.tgz --exclude=./var/lib/gusto-cloud/cloud/releases --exclude=./var/lib/gusto-cloud/kits --exclude=./var/lib/gusto-cloud/cells -C / ./var/lib/gusto-cloud ./var/www/gusto-media \$(cd / && ls -d ./var/lib/gusto/cloud.db* ./var/lib/gusto/index 2>/dev/null) && aws s3 cp /tmp/move.tgz s3://$BUCKET/$key --only-show-errors; }; then systemctl start gusto-cloud gusto-owner; exit 1; fi; rm -f /tmp/move.tgz"
aws s3 cp "s3://$BUCKET/$key" "$here/move.tgz" --only-show-errors
echo "packed $(du -h "$here/move.tgz" | cut -f1)"
scp -q "$here/move.tgz" "$HOST:/root/gusto-move.tgz"
# shellcheck disable=SC2029
ssh "$HOST" 'set -e; systemctl stop gusto-owner gusto-cloud 2>/dev/null || true
  mkdir -p /root/gusto-before-move && tar -czf "/root/gusto-before-move/$(date -u +%Y%m%dT%H%M%SZ).tgz" -C /var/lib gusto-cloud gusto 2>/dev/null || true
  tar -xzf /root/gusto-move.tgz -C / && chown -R gusto:gusto /var/lib/gusto-cloud /var/lib/gusto && rm -f /root/gusto-move.tgz
  systemctl start gusto-cloud gusto-owner && sleep 3 && systemctl is-active gusto-cloud'
echo "the data is on $HOST, and gusto cloud is running there; the front door on AWS is stopped (start it again to go back)"
