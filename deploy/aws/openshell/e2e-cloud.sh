#!/usr/bin/env bash
# The whole public cloud on this instance, on a real model: an admin signs up, invites three
# people, each gets a computer and asks an agent for something, and a second organisation
# is made beside it. Run ./up.sh first (INSTANCE_TYPE=m8i.2xlarge is enough for four
# computers), and ./put-key.sh for both keys. Output is kept in out/e2e-cloud.txt. The
# front door is left running on the instance, on port 8050.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
./image.sh runner-all-rootfs "$(./harnesses.sh)"
(cd "$root" && for program in haas:./cmd/gusto drive:./deploy/aws/openshell/e2e/cloud; do
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/${program%%:*}" "${program#*:}"
done)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e/cloud; pkill -f "[h]aas cloud"; pkill -f "[h]aas serve"; sleep 1; true'
scp -q "${ssh_options[@]}" "$state/haas" "$state/drive" e2e/cloud-on-host.sh e2e/credentials.sh "ubuntu@$address:e2e/cloud/"
ssh "${ssh_options[@]}" "ubuntu@$address" "export AWS_REGION=$AWS_REGION BEDROCK_PARAMETER=${GUSTO_MODEL_PARAMETER:-/haas/poc/model-key} DEEPSEEK_PARAMETER=${GUSTO_DEEPSEEK_PARAMETER:-/haas/poc/deepseek-key}; bash ~/e2e/cloud/cloud-on-host.sh" 2>&1 | tee out/e2e-cloud.txt
