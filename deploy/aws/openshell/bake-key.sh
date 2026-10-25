#!/usr/bin/env bash
# What names the image of a cell: the runner image, and what installs the gateway and its tools on the host. A host made from an
# image goes straight to configuring itself when the release it is made for names the same key, and does everything when it does not,
# so a change to any of this is a new image, and a host that starts from the old one before it is made is slow and not wrong.
# The format is in the key, so that a change in what is taken out of a host before its image is taken makes a new one.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
here="$root/deploy/aws/openshell"
{
  echo "bake format 4"
  "$here/image-key.sh"
  cd "$root" && git ls-files -z deploy/aws/openshell/ansible/roles/openshell deploy/aws/openshell/ansible/roles/demo/tasks/cell.yml \
    deploy/aws/openshell/ansible/roles/demo/templates/gusto-cell.service.j2 | xargs -0 shasum -a 256
} | shasum -a 256 | cut -c1-16
