#!/usr/bin/env bash
# One job through a haas server that runs jobs in OpenShell sandboxes, whose harness is the probe of deploy/smoke-isolation.sh.
# Run ./up.sh first; the output is kept in out/box-probe.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/haas" ./cmd/gusto)
sed -n "/<<'JSON'/,/^JSON/p" "$root/deploy/smoke-isolation.sh" | sed '1d;$d' > "$state/box-probe.json"
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e; pkill -f "[e]2e/haas serve" || true; pkill -f "[f]ake_model.py" || true; sleep 1; rm -f ~/e2e/haas'
scp -q "${ssh_options[@]}" "$state/haas" "$state/box-probe.json" e2e/fake_model.py "ubuntu@$address:e2e/"
ssh "${ssh_options[@]}" "ubuntu@$address" 'bash -s' < e2e/box-probe-on-host.sh 2>&1 | tee out/box-probe.txt
