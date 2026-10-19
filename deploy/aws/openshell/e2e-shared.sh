#!/usr/bin/env bash
# Three things on one gateway, on real models: two different harnesses in turn in one
# sandbox on one project, a web app the agents wrote served from the sandbox, and several
# sandboxes at once. Run ./up.sh first, and ./put-key.sh for both keys (see e2e-matrix.sh).
# Output is kept in out/e2e-shared.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
./image.sh runner-all-rootfs "$(./harnesses.sh)"
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/shared" ./deploy/aws/openshell/e2e/shared)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e/shared; rm -rf ~/e2e/shared/sample'
scp -q -r "${ssh_options[@]}" e2e/sample "ubuntu@$address:e2e/shared/"
scp -q "${ssh_options[@]}" "$state/shared" e2e/shared-on-host.sh e2e/credentials.sh "ubuntu@$address:e2e/shared/"
ssh "${ssh_options[@]}" "ubuntu@$address" "export AWS_REGION=$AWS_REGION BEDROCK_PARAMETER=${GUSTO_MODEL_PARAMETER:-/gusto/poc/model-key} DEEPSEEK_PARAMETER=${GUSTO_DEEPSEEK_PARAMETER:-/gusto/poc/deepseek-key}; bash ~/e2e/shared/shared-on-host.sh" 2>&1 | tee out/e2e-shared.txt
