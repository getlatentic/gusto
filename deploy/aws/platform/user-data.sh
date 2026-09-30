#!/bin/bash
# Runs once, as root, when a demo host is launched, and makes it a demo host of the public
# cloud from the release bundle in the store bucket: no person and no SSH is needed. The launcher
# fills in the settings on the next lines. ROLE is all (a host that is the front door and runs the
# organisations' servers), orchestrator (the front door alone) or cell (runs servers for a front door);
# the last two share the file system EFS, and RUN is the directory of it that this demo's hosts use.
set -Eeuo pipefail
RELEASE=${RELEASE:-@RELEASE@} BUCKET=${BUCKET:-@BUCKET@} REGION=${REGION:-@REGION@}
LEASE_MINUTES=${LEASE_MINUTES:-@LEASE_MINUTES@} MAX_HOURS=${MAX_HOURS:-@MAX_HOURS@}
ROLE=${ROLE:-@ROLE@} RUN=${RUN:-@RUN@} EFS=${EFS:-@EFS@}
dir=${HAAS_BUNDLE_DIR:-/opt/haas-bundle} tmp=${HAAS_TMP:-/tmp}
exec > >(tee -a "${HAAS_BOOTSTRAP_LOG:-/var/log/haas-bootstrap.log}") 2>&1

# A host that never finishes starting is still removed: the instance terminates when it powers off.
systemd-run --on-active="${MAX_HOURS}h" --unit=haas-deadman systemctl poweroff
trap 'echo "haas-bootstrap: failed, so the host powers off in 15 minutes"; systemd-run --on-active=15m --unit=haas-failed systemctl poweroff' ERR

export AWS_REGION=$REGION AWS_DEFAULT_REGION=$REGION PATH=$PATH:/snap/bin:/usr/local/bin
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq ansible unzip curl ca-certificates
command -v aws >/dev/null || snap install aws-cli --classic

mkdir -p "$dir"
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz" "$tmp/bundle.tgz" --only-show-errors
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz.sha256" "$tmp/bundle.tgz.sha256" --only-show-errors
# Nothing of the bundle is run until it is the one CI made.
(cd "$tmp" && sha256sum -c bundle.tgz.sha256)
tar -xzf "$tmp/bundle.tgz" -C "$dir"
aws ssm get-parameter --name /haas/poc/demo-config --query Parameter.Value --output text > "$dir/demo-vars.json"

# A front door alone runs no computer, so it has no use for OpenShell.
playbooks=("$dir/ansible/site.yml" "$dir/ansible/demo.yml")
[ "$ROLE" != orchestrator ] || playbooks=("$dir/ansible/demo.yml")
shared=()
[ "$ROLE" = all ] || shared=(-e "demo_efs=$EFS.efs.$REGION.amazonaws.com" -e "demo_run=$RUN")

ansible-playbook -c local -i 'spike,' \
  -e "@$dir/demo-vars.json" -e "demo_bucket=$BUCKET" -e "demo_region=$REGION" -e "demo_image_key=$(cat "$dir/image.key")" \
  -e "demo_lease_minutes=$LEASE_MINUTES" -e "demo_max_hours=$MAX_HOURS" -e demo_check_public=false -e "demo_role=$ROLE" ${shared[@]+"${shared[@]}"} \
  "${playbooks[@]}"
echo "haas-bootstrap: done"
