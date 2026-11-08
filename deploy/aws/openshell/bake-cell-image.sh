#!/usr/bin/env bash
# Makes the machine image a host of the live gusto cloud starts from, and points /gusto/poc/cell-ami at it. A cold start spends its first
# minutes installing packages and running the playbook, and then, once per host, the gateway turns the 1.7 GB runner image into the disk
# a sandbox starts from, which was 139 of 270 seconds. A host made from this image has all of that done and goes straight to
# configuring itself from the release it is told to be.
#
# It makes a host as gusto cloud would, except that it is not started as one, and the gateway is asked for its first sandbox
# (gusto scan), which prepares the disk. It then takes an image of it and removes it. About fifteen minutes of an m8i.xlarge,
# and about 0.60 dollars a month of snapshot while the image is kept. What goes into the image is in its key, which is in the
# image's tags, so this does nothing when the image of the same key is there: FORCE=1 makes it again.
set -euo pipefail
cd "$(dirname "$0")"
: "${AWS_PROFILE:=gusto-poc}" "${AWS_REGION:=us-east-1}"
export AWS_PROFILE AWS_REGION AWS_DEFAULT_REGION="$AWS_REGION"
root="$(git rev-parse --show-toplevel)"
param=/gusto/poc/cell-ami

key="$(./bake-key.sh)"

[ -z "${ONLY_KEY:-}" ] || { echo "$key"; exit 0; }

# An image takes a quarter of an hour to be made from a 40 GB disk, and the default wait is ten minutes, so it is waited for here.
wait_for_image() {
  local state
  for _ in $(seq 1 270); do
    state="$(aws ec2 describe-images --image-ids "$1" --query 'Images[0].State' --output text)"
    case "$state" in available) return 0 ;; failed) echo "AWS could not make the image $1" >&2; return 1 ;; esac
    sleep 10
  done
  echo "the image $1 was not made in 45 minutes" >&2
  return 1
}

existing="$(aws ec2 describe-images --owners self --filters "Name=tag:gusto-bake-key,Values=$key" Name=state,Values=available,pending --query 'sort_by(Images,&CreationDate)[-1].ImageId' --output text)"
if [ "$existing" != None ] && [ -z "${FORCE:-}" ]; then
  wait_for_image "$existing"
  aws ssm put-parameter --name "$param" --type String --data-type aws:ec2:image --overwrite --value "$existing" >/dev/null
  echo "the image $existing has this key, $key, and the host template uses it"
  exit 0
fi

# CI names the bucket; making it and closing it to the public is for whoever sets the cloud up, which CI is not.
bucket="${BUCKET:-$(./demo-store.sh)}"
[[ "$bucket" =~ ^gusto-poc-demo-[0-9]{12}$ ]] || { echo "demo-store.sh did not give a bucket name: $bucket" >&2; exit 1; }
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
host="$(aws ec2 run-instances --launch-template LaunchTemplateName=gusto-poc-demo-host-cell-live,Version='$Latest' --image-id "$base" \
  --user-data "file://$userdata" \
  --tag-specifications "ResourceType=instance,Tags=[{Key=project,Value=gusto-poc},{Key=name,Value=cell-bake}]" "ResourceType=volume,Tags=[{Key=project,Value=gusto-poc},{Key=name,Value=cell-bake}]" \
  --query 'Instances[0].InstanceId' --output text)"
echo "the host to make the image from is $host"

run() { # run a command on the host with Systems Manager and print what it said
  local id status
  id="$(aws ssm send-command --instance-ids "$host" --document-name AWS-RunShellScript --parameters "commands=[\"$1\"]" --query Command.CommandId --output text 2>/dev/null)" || return 1
  for _ in $(seq 1 60); do
    status="$(aws ssm get-command-invocation --command-id "$id" --instance-id "$host" --query Status --output text 2>/dev/null || true)"
    case "$status" in Success) aws ssm get-command-invocation --command-id "$id" --instance-id "$host" --query StandardOutputContent --output text; return 0 ;; Failed|Canceling|TimedOut) return 1 ;; esac
    sleep 5
  done
  return 1
}

printf 'waiting for it to be made a host and for the gateway to prepare the runner image '
for _ in $(seq 1 90); do
  if [ "$(run 'test -e /etc/gusto/bake-done && echo done' 2>/dev/null | tr -d '[:space:]')" = done ]; then echo; break; fi
  printf '.'; sleep 10
done
run 'test -e /etc/gusto/bake-done' >/dev/null || { echo "the host was not made in time: see /var/log/gusto-bootstrap.log on $host" >&2; exit 1; }

# What is the host's own is taken out, so that a host made from the image is a new one: it fetches its own secrets and takes its own name.
# The file system is taken out of fstab too: a host that has it there before it has its network orders it before the network, and
# systemd breaks the cycle by deleting the network jobs, which cost a host a minute and a half. The playbook mounts it again.
# The start-up script is turned off: run with the settings of the host the image was made from, it held back the playbook, which
# runs it with the new host's own. cloud-init is turned off too, since its Python cost a new host seconds of a disk still being
# fetched: gusto-launch runs the host's script in its place, and the network is one for any interface, not the one cloud-init
# wrote for this host's. sshd is taken out with its host keys: cloud-init made new keys on a host's first boot, and without it every
# host of the image would share them. A cell is reached through Systems Manager and its security group has no SSH open.
run "systemctl stop openshell-gateway gusto-boot 2>/dev/null; systemctl disable gusto-boot 2>/dev/null; systemctl disable --now ssh.socket ssh.service 2>/dev/null; rm -f /etc/ssh/ssh_host_*; install -m 0600 /usr/local/lib/gusto/netplan-cell.yaml /etc/netplan/10-cell.yaml && rm -f /etc/netplan/50-cloud-init.yaml && touch /etc/cloud/cloud-init.disabled; umount /var/lib/gusto-cloud /mnt/efs 2>/dev/null; grep -vE 'mnt/efs|var/lib/gusto-cloud' /etc/fstab > /tmp/fstab.new; cat /tmp/fstab.new > /etc/fstab; printf '%s\\n' '$key' > /etc/gusto/baked; rm -f /etc/gusto/bake-done /var/log/gusto-bootstrap.log; cloud-init clean --logs; truncate -s 0 /etc/machine-id; sync; ! ls /etc/ssh/ssh_host_* >/dev/null 2>&1 && echo cleaned" | grep -q cleaned
aws ec2 stop-instances --instance-ids "$host" >/dev/null
aws ec2 wait instance-stopped --instance-ids "$host"
name="gusto-cell-$key-$(date -u +%Y%m%d%H%M)"
image="$(aws ec2 create-image --instance-id "$host" --name "$name" --description "A host of the public cloud with its runner image prepared" \
  --tag-specifications "ResourceType=image,Tags=[{Key=gusto-bake-key,Value=$key},{Key=project,Value=gusto-poc}]" "ResourceType=snapshot,Tags=[{Key=gusto-bake-key,Value=$key},{Key=project,Value=gusto-poc}]" \
  --query ImageId --output text)"
echo "making the image $image"
wait_for_image "$image"
aws ssm put-parameter --name "$param" --type String --data-type aws:ec2:image --overwrite --value "$image" >/dev/null

# Two images are kept, the new one and the one before it, which a host that is starting may still be made from.
aws ec2 describe-images --owners self --filters 'Name=tag-key,Values=gusto-bake-key' --query 'sort_by(Images,&CreationDate)[:-2].[ImageId]' --output text | while read -r old; do
  [ -n "$old" ] || continue
  snapshots="$(aws ec2 describe-images --image-ids "$old" --query 'Images[0].BlockDeviceMappings[].Ebs.SnapshotId' --output text)"
  aws ec2 deregister-image --image-id "$old" >/dev/null
  for snapshot in $snapshots; do aws ec2 delete-snapshot --snapshot-id "$snapshot" >/dev/null 2>&1 || true; done
  echo "removed the older image $old"
done
echo "made $image from key $key in $(( ($(date +%s) - started) / 60 )) minutes, and the host template uses it"
