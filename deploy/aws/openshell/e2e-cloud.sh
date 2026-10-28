#!/usr/bin/env bash
# The whole public cloud on this instance, on a real model: an admin signs up, invites three
# people, each gets a computer and asks an agent for something, and a second organization
# is made beside it. Run ./up.sh first (INSTANCE_TYPE=m8i.2xlarge is enough for four
# computers), and ./put-key.sh for both keys. Output is kept in out/e2e-cloud.txt. The
# front door is left running on the instance, on port 8050. SANDBOX=container runs the agents in Docker containers instead of
# OpenShell sandboxes.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
./image.sh runner-all-rootfs "$(./harnesses.sh)"
# With SANDBOX=container the agents run in Docker, from the image the root file system was exported from.
if [ "${SANDBOX:-openshell}" = container ]; then
  ssh "${ssh_options[@]}" "ubuntu@$address" "set -e
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker.io >/dev/null 2>&1
    sudo usermod -aG docker ubuntu
    sudo podman save --format docker-archive docker.io/library/gusto-runner | sudo docker load -q
    sudo docker tag gusto-runner:latest gusto-runner:e2e"
fi
(cd "$root" && for program in gusto:./cmd/gusto drive:./deploy/aws/openshell/e2e/cloud; do
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/${program%%:*}" "${program#*:}"
done)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e/cloud; pkill -f "[g]usto cloud"; pkill -f "[g]usto serve"; sleep 1; true'
scp -q "${ssh_options[@]}" "$state/gusto" "$state/drive" e2e/cloud-on-host.sh e2e/credentials.sh e2e/isolation-on-host.sh "ubuntu@$address:e2e/cloud/"
ssh "${ssh_options[@]}" "ubuntu@$address" "export SANDBOX=${SANDBOX:-openshell} MODEL=${MODEL:-} AWS_REGION=$AWS_REGION BEDROCK_PARAMETER=${GUSTO_MODEL_PARAMETER:-/gusto/poc/model-key} DEEPSEEK_PARAMETER=${GUSTO_DEEPSEEK_PARAMETER:-/gusto/poc/deepseek-key}; bash ~/e2e/cloud/cloud-on-host.sh" 2>&1 | tee out/e2e-cloud.txt
