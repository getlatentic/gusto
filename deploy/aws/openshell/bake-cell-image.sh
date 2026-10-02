#!/usr/bin/env bash
# Makes the machine image a cell of the live front door starts from, and points /haas/poc/cell-ami at it. A cold start spends its first
# minutes installing packages and running the playbook, and then, once per host, the gateway turns the 1.7 GB runner image into the disk
# a sandbox starts from, which was 139 of 270 seconds. A host made from this image has all of that done and goes straight to
# configuring itself from the release it is told to be.
#
# It makes a cell as the front door would, except that it is not started as one, and the gateway is asked for its first sandbox
# (haas survey), which prepares the disk. It then takes an image of it and removes it. About fifteen minutes of an m8i.xlarge,
# and about 0.60 dollars a month of snapshot while the image is kept. What goes into the image is in its key, which is in the
# image's tags, so this does nothing when the image of the same key is there: FORCE=1 makes it again.
set -euo pipefail
cd "$(dirname "$0")"
: "${AWS_PROFILE:=haas-poc}" "${AWS_REGION:=us-east-1}"
export AWS_PROFILE AWS_REGION AWS_DEFAULT_REGION="$AWS_REGION"
root="$(git rev-parse --show-toplevel)"
param=/haas/poc/cell-ami

# What is in the image: the runner image, the playbook that installs the gateway and its tools, and this script.
key="$({ ./image-key.sh; (cd "$root" && git ls-files -z deploy/aws/openshell/ansible/roles/openshell deploy/aws/platform/user-data.sh deploy/aws/openshell/bake-cell-image.sh deploy/aws/openshell/ansible/roles/demo/tasks/cell.yml deploy/aws/openshell/ansible/roles/demo/templates/haas-cell.service.j2 | xargs -0 shasum -a 256); } | shasum -a 256 | cut -c1-16)"

existing="$(aws ec2 describe-images --owners self --filters "Name=tag:haas-bake-key,Values=$key" Name=state,Values=available --query 'Images[0].ImageId' --output text)"
if [ "$existing" != None ] && [ -z "${FORCE:-}" ]; then
  aws ssm put-parameter --name "$param" --type String --data-type aws:ec2:image --overwrite --value "$existing" >/dev/null
  echo "the image $existing has this key, $key, and the cell template uses it"
  exit 0
fi

bucket="$(./demo-store.sh)"
[[ "$bucket" =~ ^haas-poc-demo-[0-9]{12}$ ]] || { echo "demo-store.sh did not give a bucket name: $bucket" >&2; exit 1; }
efs="$(aws efs describe-file-systems --query 'FileSystems[0].FileSystemId' --output text)"
base="$(aws ssm get-parameter --name /aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id --query Parameter.Value --output text)"

userdata="$(mktemp)"
{
  echo '#!/bin/bash'
  echo "export RELEASE=latest BUCKET=$bucket REGION=$AWS_REGION LEASE_MINUTES=30 MAX_HOURS=2 ROLE=cell RUN=live EFS=$efs LIVE=off BAKE=on"
  cat "$root/deploy/aws/platform/user-data.sh"
} > "$userdata"

host=""
cleanup() { [ -z "$host" ] || aws ec2 terminate-instances --instance-ids "$host" >/dev/null 2>&1 || true; rm -f "$userdata"; }
trap cleanup EXIT

started="$(date +%s)"
host="$(aws ec2 run-instances --launch-template LaunchTemplateName=haas-poc-demo-host-cell-live,Version='$Latest' --image-id "$base" \
  --user-data "file://$userdata" \
  --tag-specifications "ResourceType=instance,Tags=[{Key=project,Value=haas-poc},{Key=name,Value=cell-bake}]" \
  --query 'Instances[0].InstanceId' --output text)"
echo "the host to make the image from is $host"

run() { # run a command on the host with Systems Manager and print what it said
  local id status
  id="$(aws ssm send-command --instance-ids "$host" --document-name AWS-RunShellScript --parameters "commands=[\"$1\"]" --query Command.CommandId --output text 2>/dev/null)" || return 1
  for _ in $(seq 1 60); do
    status="$(aws ssm get-command-invocation --command-id "$id" --instance-id "$host" --query Status --output text 2>/dev/null || true)"
    case "$status" in Success) aws ssm get-command-invocation --command-id "$id" --instance-id "$host" --query StandardOutputContent --output text; return 0 ;; Failed|Cancelled|TimedOut) return 1 ;; esac
    sleep 5
  done
  return 1
}

printf 'waiting for it to be made a cell and for the gateway to prepare the runner image '
for _ in $(seq 1 90); do
  if [ "$(run 'test -e /etc/haas/bake-done && echo done' 2>/dev/null | tr -d '[:space:]')" = done ]; then echo; break; fi
  printf '.'; sleep 10
done
run 'test -e /etc/haas/bake-done' >/dev/null || { echo "the host was not made in time: see /var/log/haas-bootstrap.log on $host" >&2; exit 1; }

# What is the host's own is taken out, so that a host made from the image is a new one: it fetches its own secrets and takes its own name.
run "systemctl stop openshell-gateway haas-boot 2>/dev/null; printf '%s\\n' '$key' > /etc/haas/baked; rm -f /etc/haas/bake-done /var/log/haas-bootstrap.log; cloud-init clean --logs; truncate -s 0 /etc/machine-id; sync; echo cleaned" | grep -q cleaned
aws ec2 stop-instances --instance-ids "$host" >/dev/null
aws ec2 wait instance-stopped --instance-ids "$host"
name="haas-cell-$key-$(date -u +%Y%m%d%H%M)"
image="$(aws ec2 create-image --instance-id "$host" --name "$name" --description "A cell of the public cloud with its runner image prepared" \
  --tag-specifications "ResourceType=image,Tags=[{Key=haas-bake-key,Value=$key},{Key=project,Value=haas-poc}]" "ResourceType=snapshot,Tags=[{Key=haas-bake-key,Value=$key},{Key=project,Value=haas-poc}]" \
  --query ImageId --output text)"
echo "making the image $image"
aws ec2 wait image-available --image-ids "$image"
aws ssm put-parameter --name "$param" --type String --data-type aws:ec2:image --overwrite --value "$image" >/dev/null

# Two images are kept, the new one and the one before it, which a cell that is starting may still be made from.
aws ec2 describe-images --owners self --filters 'Name=tag-key,Values=haas-bake-key' --query 'sort_by(Images,&CreationDate)[:-2].[ImageId]' --output text | while read -r old; do
  [ -n "$old" ] || continue
  snapshots="$(aws ec2 describe-images --image-ids "$old" --query 'Images[0].BlockDeviceMappings[].Ebs.SnapshotId' --output text)"
  aws ec2 deregister-image --image-id "$old" >/dev/null
  for snapshot in $snapshots; do aws ec2 delete-snapshot --snapshot-id "$snapshot" >/dev/null 2>&1 || true; done
  echo "removed the older image $old"
done
echo "made $image from key $key in $(( ($(date +%s) - started) / 60 )) minutes, and the cell template uses it"
