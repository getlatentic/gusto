#!/usr/bin/env bash
# Builds the runner image on the instance, with the harnesses named, and exports
# it as the root file system a sandbox is made from: ./image.sh <name> "<name=package ...>".
# The instance is x86-64 like the sandboxes it starts, and a machine of another
# architecture would need emulation to run the image's build. An image that is
# already there is kept; set REBUILD=1 to build it again.
source "$(dirname "$0")/env.sh"
name="${1:?a name for the root file system}"
harnesses="${2:?the harnesses, as name=package}"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
if [ -z "${REBUILD:-}" ] && ssh "${ssh_options[@]}" "ubuntu@$address" "test -s ~/e2e/$name.tar.gz"; then exit 0; fi
git -C "$root" ls-files -z | COPYFILE_DISABLE=1 tar --no-xattrs --null -T - -czf "$state/src.tgz" -C "$root"
ssh "${ssh_options[@]}" "ubuntu@$address" 'rm -rf ~/src; mkdir -p ~/e2e ~/src'
scp -q "${ssh_options[@]}" "$state/src.tgz" "ubuntu@$address:src.tgz"
ssh "${ssh_options[@]}" "ubuntu@$address" "set -e
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq podman >/dev/null 2>&1
  tar xzf ~/src.tgz -C ~/src && cd ~/src
  sudo podman build -q --build-arg 'HARNESSES=$harnesses' -t docker.io/library/gusto-runner . >/dev/null
  cid=\$(sudo podman create docker.io/library/gusto-runner true)
  sudo podman export \"\$cid\" | gzip -1 > ~/e2e/$name.tar.gz
  sudo podman rm \"\$cid\" >/dev/null
  ls -la ~/e2e/$name.tar.gz"
