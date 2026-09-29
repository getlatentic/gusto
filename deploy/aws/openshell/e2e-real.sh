#!/usr/bin/env bash
# opencode, a real harness, on the sample project in an OpenShell sandbox made
# from the runner image. Run ./up.sh first.
#
# With no model settings it runs against a model that only answers "pong", which
# shows the harness starts in the sandbox, reaches haas's proxy, and is denied
# everything else. With HAAS_MODEL_UPSTREAM and HAAS_MODEL_KEY set in the
# environment of the person running this, the same job runs on that model: the
# key goes to the instance over ssh's standard input into a file only its user
# can read, and is never an argument or written here. The instance is
# destroyed by ./down.sh. Output is kept in out/e2e-real.txt.
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
if [ -n "${HAAS_MODEL_KEY:-}" ]; then
  : "${HAAS_MODEL_UPSTREAM:?set HAAS_MODEL_UPSTREAM with HAAS_MODEL_KEY}"
  printf 'HAAS_MODEL_UPSTREAM=%s\nHAAS_MODEL_KEY=%s\n' "$HAAS_MODEL_UPSTREAM" "$HAAS_MODEL_KEY" \
    | ssh "${ssh_options[@]}" "ubuntu@$address" 'umask 077; cat > ~/e2e/model.env'
fi
ssh "${ssh_options[@]}" "ubuntu@$address" 'bash -s' < e2e/real-on-host.sh 2>&1 | tee out/e2e-real.txt
