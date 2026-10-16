#!/usr/bin/env bash
# A haas server on the instance whose jobs run in OpenShell sandboxes, and one
# job through it: the sandbox reaches the model through haas's proxy and no
# other host, and what its guard allowed and denied is in the signed record.
# Run ./up.sh first; the output is kept in out/e2e.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/haas" ./cmd/gusto)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e; pkill -f "[e]2e/haas serve" || true; pkill -f "[f]ake_model.py" || true; sleep 1; rm -f ~/e2e/haas'
scp -q "${ssh_options[@]}" "$state/haas" e2e/fake_model.py e2e/inside-harness.sh "ubuntu@$address:e2e/"
ssh "${ssh_options[@]}" "ubuntu@$address" 'bash -s' < e2e/on-host.sh 2>&1 | tee out/e2e.txt
