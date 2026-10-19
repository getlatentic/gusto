#!/usr/bin/env bash
# Destroys everything up.sh made, then asks AWS whether anything of it is
# still there.
source "$(dirname "$0")/env.sh"
terraform init -input=false >/dev/null
terraform destroy -input=false -auto-approve
# Each service that bills is asked directly. The tag search is not, because it
# lists deleted resources for some time after they are gone.
left="$(
  aws ec2 describe-instances --filters Name=tag:project,Values=gusto-poc \
    Name=instance-state-name,Values=pending,running,stopping,stopped \
    --query 'Reservations[].Instances[].InstanceId' --output text
  aws ec2 describe-addresses --filters Name=tag:project,Values=gusto-poc --query 'Addresses[].PublicIp' --output text
  aws ec2 describe-volumes --filters Name=tag:project,Values=gusto-poc --query 'Volumes[].VolumeId' --output text
  aws ec2 describe-vpcs --filters Name=tag:project,Values=gusto-poc --query 'Vpcs[].VpcId' --output text
  aws s3api list-buckets --query 'Buckets[?starts_with(Name, `gusto-poc`)].Name' --output text
  aws autoscaling describe-auto-scaling-groups --auto-scaling-group-names gusto-poc \
    --query 'AutoScalingGroups[].AutoScalingGroupName' --output text
)"
left="$(echo "$left" | tr '\t' '\n' | grep -v '^\s*$' || true)"
if [ -n "$left" ]; then
  echo "still there, and billing:" >&2; echo "$left" >&2; exit 1
fi
echo "down: no instance, address, disk, network, bucket or group of the POC is left"
