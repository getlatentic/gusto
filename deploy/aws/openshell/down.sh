#!/usr/bin/env bash
# Destroys everything up.sh made, then asks AWS whether anything of it is still
# there. The local key goes with it.
source "$(dirname "$0")/env.sh"
terraform init -input=false >/dev/null
terraform destroy -input=false -auto-approve
left="$(
  aws ec2 describe-instances --filters Name=tag:project,Values=haas-poc \
    Name=instance-state-name,Values=pending,running,stopping,stopped \
    --query 'Reservations[].Instances[].InstanceId' --output text
  aws ec2 describe-volumes --filters Name=tag:project,Values=haas-poc --query 'Volumes[].VolumeId' --output text
  aws ec2 describe-security-groups --filters Name=tag:project,Values=haas-poc --query 'SecurityGroups[].GroupId' --output text
  aws ec2 describe-key-pairs --filters Name=tag:project,Values=haas-poc --query 'KeyPairs[].KeyName' --output text
  aws ec2 describe-addresses --filters Name=tag:project,Values=haas-poc --query 'Addresses[].PublicIp' --output text
)"
left="$(echo "$left" | tr '\t' '\n' | grep -v '^\s*$' || true)"
if [ -n "$left" ]; then
  echo "still there, and billing:" >&2; echo "$left" >&2; exit 1
fi
rm -rf "$state"
echo "down: no instance, disk, group, key or address of the test is left"
