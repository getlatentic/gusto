#!/usr/bin/env bash
# Destroys everything up.sh made, then asks AWS whether anything of it is still
# there. The local key goes with it. A resource of the platform stack, which carries the
# same project tag and a stack tag, is the front door's and is not counted.
source "$(dirname "$0")/env.sh"
terraform init -input=false >/dev/null
terraform destroy -input=false -auto-approve
left="$(
  aws ec2 describe-instances --filters Name=tag:project,Values=gusto-poc \
    Name=instance-state-name,Values=pending,running,stopping,stopped \
    --query 'Reservations[].Instances[?!(Tags[?Key==`stack`])].InstanceId' --output text
  aws ec2 describe-volumes --filters Name=tag:project,Values=gusto-poc --query 'Volumes[?!(Tags[?Key==`stack`])].VolumeId' --output text
  aws ec2 describe-security-groups --filters Name=tag:project,Values=gusto-poc --query 'SecurityGroups[?!(Tags[?Key==`stack`])].GroupId' --output text
  aws ec2 describe-key-pairs --filters Name=tag:project,Values=gusto-poc --query 'KeyPairs[?!(Tags[?Key==`stack`])].KeyName' --output text
  aws ec2 describe-addresses --filters Name=tag:project,Values=gusto-poc --query 'Addresses[?!(Tags[?Key==`stack`])].PublicIp' --output text
)"
left="$(echo "$left" | tr '\t' '\n' | grep -v '^\s*$' || true)"
if [ -n "$left" ]; then
  echo "still there, and billing:" >&2; echo "$left" >&2; exit 1
fi
rm -rf "$state"
echo "down: no instance, disk, group, key or address of the test is left"
