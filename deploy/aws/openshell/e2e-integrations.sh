#!/usr/bin/env bash
# The integrations epic's demo, on this backend: an integration imported through the console, turned on, given to a skill set, and
# Claude Code and Codex each used it from an OpenShell sandbox, with each tool call in the job's record. Run ./up.sh first, and
# ./put-key.sh for /gusto/poc/model-key and, with GUSTO_MODEL_PARAMETER=/gusto/poc/deepseek-key, DeepSeek's. The instance reads the keys
# itself. Output is kept in out/e2e-integrations.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
harnesses="$(./harnesses.sh)"
./image.sh runner-all-rootfs "$harnesses"
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/gusto" ./cmd/gusto)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e/integ; pkill -f "[e]2e/gusto (serve|matrix)" || true; sleep 1; rm -f ~/e2e/gusto'
scp -q "${ssh_options[@]}" "$state/gusto" "ubuntu@$address:e2e/"
scp -q -r "${ssh_options[@]}" e2e/sample "ubuntu@$address:e2e/"
scp -q "${ssh_options[@]}" e2e/integrations-on-host.sh e2e/customers_server.py e2e/credentials.sh "ubuntu@$address:e2e/"
ssh "${ssh_options[@]}" "ubuntu@$address" "export AWS_REGION=$AWS_REGION BEDROCK_PARAMETER=${GUSTO_MODEL_PARAMETER:-/gusto/poc/model-key} DEEPSEEK_PARAMETER=${GUSTO_DEEPSEEK_PARAMETER:-/gusto/poc/deepseek-key}
  rm -f ~/e2e/integ/done; setsid nohup bash ~/e2e/integrations-on-host.sh > ~/e2e/integ-host.log 2>&1 < /dev/null &"
{
  until ssh "${ssh_options[@]}" "ubuntu@$address" 'test -f ~/e2e/integ/done'; do
    ssh "${ssh_options[@]}" "ubuntu@$address" 'tail -n 2 ~/e2e/integ-host.log 2>/dev/null' | tr '\n' ' '; echo
    sleep 30
  done
  ssh "${ssh_options[@]}" "ubuntu@$address" 'cat ~/e2e/integ-host.log'
} 2>&1 | tee out/e2e-integrations.txt
