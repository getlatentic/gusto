#!/usr/bin/env bash
# One comparison, started from the pages' own form, on a haas server whose jobs
# run in OpenShell sandboxes made from a Node root file system. Run ./up.sh
# first. The root file system is made from node:24-slim with docker or podman on
# this machine; the output is kept in out/e2e-compare.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
runtime="$(command -v docker || command -v podman)"
[ -f "$state/node-rootfs.tar.gz" ] || {
  # The instance is x86-64, whatever this machine is.
  "$runtime" pull -q --platform linux/amd64 docker.io/library/node:24-slim >/dev/null
  cid="$("$runtime" create --platform linux/amd64 docker.io/library/node:24-slim true)"
  "$runtime" export "$cid" | gzip -1 > "$state/node-rootfs.tar.gz"
  "$runtime" rm "$cid" >/dev/null
}
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/haas" ./cmd/gusto)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e; pkill -f "[e]2e/haas serve" || true; pkill -f "[f]ake_model.py" || true; sleep 1; rm -f ~/e2e/haas'
scp -q "${ssh_options[@]}" "$state/haas" "$state/node-rootfs.tar.gz" e2e/fake_model.py "ubuntu@$address:e2e/"
ssh "${ssh_options[@]}" "ubuntu@$address" 'python3 ~/e2e/fake_model.py >/dev/null 2>&1 </dev/null & disown; true'
ssh "${ssh_options[@]}" "ubuntu@$address" 'bash -s' < e2e/compare-on-host.sh 2>&1 | tee out/e2e-compare.txt
