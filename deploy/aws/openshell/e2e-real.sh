#!/usr/bin/env bash
# opencode, a real harness, on the sample project in an OpenShell sandbox made
# from the runner image. Run ./up.sh first.
#
# With no model settings it runs against a model that only answers "pong", which
# shows the harness starts in the sandbox, reaches haas's proxy, and is denied
# everything else. With HAAS_MODEL_UPSTREAM set the same job runs on that model,
# and the instance reads its key from Parameter Store with its own role, where
# ./put-key.sh stored it: the key is never here, never an argument, and lands on
# the instance only in a file its user alone can read. (HAAS_MODEL_KEY in the
# environment is still accepted, and goes over ssh's standard input.) The
# instance is destroyed by ./down.sh. Output is kept in out/e2e-real.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
# The runner image is built on the instance, which is x86-64 like the sandboxes; a
# machine of another architecture would need emulation to run the image's build.
if ! ssh "${ssh_options[@]}" "ubuntu@$address" 'test -s ~/e2e/runner-rootfs.tar.gz'; then
  git -C "$root" ls-files -z | COPYFILE_DISABLE=1 tar --no-xattrs --null -T - -czf "$state/src.tgz" -C "$root"
  ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e ~/src'
  scp -q "${ssh_options[@]}" "$state/src.tgz" "ubuntu@$address:src.tgz"
  ssh "${ssh_options[@]}" "ubuntu@$address" 'set -e
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq podman >/dev/null 2>&1
    tar xzf ~/src.tgz -C ~/src && cd ~/src
    sudo podman build -q --build-arg "HARNESSES=opencode=opencode-ai pi=@earendil-works/pi-coding-agent" -t docker.io/library/haas-runner . >/dev/null
    cid=$(sudo podman create docker.io/library/haas-runner true)
    sudo podman export "$cid" | gzip -1 > ~/e2e/runner-rootfs.tar.gz
    sudo podman rm "$cid" >/dev/null
    ls -la ~/e2e/runner-rootfs.tar.gz'
fi
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/haas" ./cmd/haas)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e; pkill -f "[e]2e/haas serve" || true; pkill -f "[f]ake_model.py" || true; sleep 1; rm -f ~/e2e/haas ~/e2e/model.env'
scp -q "${ssh_options[@]}" "$state/haas" e2e/fake_model.py "ubuntu@$address:e2e/"
scp -q -r "${ssh_options[@]}" e2e/sample "ubuntu@$address:e2e/"
if [ -n "${HAAS_MODEL_UPSTREAM:-}" ] && [ -z "${HAAS_MODEL_KEY:-}" ]; then
  # The instance reads the key itself, with its role, from where ./put-key.sh stored it.
  parameter="${HAAS_MODEL_PARAMETER:-/haas/poc/model-key}"
  ssh "${ssh_options[@]}" "ubuntu@$address" "umask 077; export AWS_REGION=$AWS_REGION
    key=\$(aws ssm get-parameter --with-decryption --name '$parameter' --query Parameter.Value --output text) || exit 1
    printf 'HAAS_MODEL_UPSTREAM=%s\nHAAS_MODEL_KEY=%s\n' '$HAAS_MODEL_UPSTREAM' \"\$key\" > ~/e2e/model.env"
elif [ -n "${HAAS_MODEL_KEY:-}" ]; then
  : "${HAAS_MODEL_UPSTREAM:?set HAAS_MODEL_UPSTREAM with HAAS_MODEL_KEY}"
  printf 'HAAS_MODEL_UPSTREAM=%s\nHAAS_MODEL_KEY=%s\n' "$HAAS_MODEL_UPSTREAM" "$HAAS_MODEL_KEY" \
    | ssh "${ssh_options[@]}" "ubuntu@$address" 'umask 077; cat > ~/e2e/model.env'
fi
ssh "${ssh_options[@]}" "ubuntu@$address" 'bash -s' < e2e/real-on-host.sh 2>&1 | tee out/e2e-real.txt
