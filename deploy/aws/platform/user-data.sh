#!/bin/bash
# Runs once, as root, when a demo host is launched, and makes it a demo host of the public
# cloud from the release bundle in the store bucket: no person and no SSH is needed. The launcher
# fills in the settings on the next lines. ROLE is all (a host that is the front door and runs the
# organisations' servers), orchestrator (the front door alone) or cell (runs servers for a front door);
# the last two share the file system EFS, and RUN is the directory of it that this demo's hosts use.
# BAKE is on for the host an image of a cell is made from: it is made as a cell is, its runner image is prepared for the gateway,
# and it is not started as a cell, so that the image holds everything a cell has to wait for and nothing that is its own. A host made
# from that image has /etc/haas/baked and goes straight to configuring itself.
# LIVE is on for the front door that stays up: it has no lease and no limit on how long it runs, keeps what
# is on the file system, starts hosts for cells when organisations need one, and is brought up to a newer
# release by running this again (HAAS_LIVE_ENV names the file that keeps what it was started with).
set -Eeuo pipefail
RELEASE=${RELEASE:-@RELEASE@} BUCKET=${BUCKET:-@BUCKET@} REGION=${REGION:-@REGION@}
LEASE_MINUTES=${LEASE_MINUTES:-@LEASE_MINUTES@} MAX_HOURS=${MAX_HOURS:-@MAX_HOURS@}
ROLE=${ROLE:-@ROLE@} RUN=${RUN:-@RUN@} EFS=${EFS:-@EFS@} LIVE=${LIVE:-@LIVE@} BAKE=${BAKE:-off}
baked_marker=${HAAS_BAKED:-/etc/haas/baked}
live_env=${HAAS_LIVE_ENV:-/etc/haas/live.env}
dir=${HAAS_BUNDLE_DIR:-/opt/haas-bundle} tmp=${HAAS_TMP:-/tmp}
exec > >(tee -a "${HAAS_BOOTSTRAP_LOG:-/var/log/haas-bootstrap.log}") 2>&1

# A host that never finishes starting is still removed: the instance terminates when it powers off. One that
# is meant to stay up is not, since it is looked at and started again rather than lost.
if [ "$LIVE" != on ]; then
  systemd-run --on-active="${MAX_HOURS}h" --unit=haas-deadman systemctl poweroff
  trap 'echo "haas-bootstrap: failed, so the host powers off in 15 minutes"; systemd-run --on-active=15m --unit=haas-failed systemctl poweroff' ERR
fi

# A small host needs room for the installs of a start and of an upgrade, which the memory it runs in does not leave.
if [ ! -e "${HAAS_SWAPFILE:-/swapfile}" ] && [ "$(awk '/MemTotal/ {print int($2/1024)}' "${HAAS_MEMINFO:-/proc/meminfo}")" -lt 3000 ]; then
  swapfile=${HAAS_SWAPFILE:-/swapfile}
  fallocate -l 1G "$swapfile" && chmod 600 "$swapfile" && mkswap "$swapfile" >/dev/null && swapon "$swapfile"
  grep -q "^$swapfile " "${HAAS_FSTAB:-/etc/fstab}" || echo "$swapfile none swap sw 0 0" >> "${HAAS_FSTAB:-/etc/fstab}"
fi

export HOME=${HOME:-/root}
export AWS_REGION=$REGION AWS_DEFAULT_REGION=$REGION PATH=$PATH:/snap/bin:/usr/local/bin
export DEBIAN_FRONTEND=noninteractive
# A host made from the image of a cell has these, and the packages are not fetched again.
if [ ! -e "$baked_marker" ]; then
  apt-get update -qq
  apt-get install -y -qq ansible unzip curl ca-certificates
  command -v aws >/dev/null || snap install aws-cli --classic
fi

# The newest release is the one CI last made, for a host that is started by hand.
[ "$RELEASE" != latest ] || RELEASE=$(aws s3 cp "s3://$BUCKET/releases/latest" -)

mkdir -p "$dir"
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz" "$tmp/bundle.tgz" --only-show-errors
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz.sha256" "$tmp/bundle.tgz.sha256" --only-show-errors
# Nothing of the bundle is run until it is the one CI made.
(cd "$tmp" && sha256sum -c bundle.tgz.sha256)
tar -xzf "$tmp/bundle.tgz" -C "$dir"
# The AWS command, a snap, writes nothing to a file it is redirected to when it is run over SSM, so it is read
# into a variable.
demo_vars=$(aws ssm get-parameter --name /haas/poc/demo-config --query Parameter.Value --output text)
printf '%s\n' "$demo_vars" > "$dir/demo-vars.json"

if [ "$LIVE" = on ]; then
  mkdir -p "$(dirname "$live_env")"
  printf 'BUCKET=%s\nREGION=%s\nROLE=%s\nRUN=%s\nEFS=%s\nLIVE=on\nLEASE_MINUTES=%s\nMAX_HOURS=%s\n' "$BUCKET" "$REGION" "$ROLE" "$RUN" "$EFS" "$LEASE_MINUTES" "$MAX_HOURS" > "$live_env"
fi

# A front door alone runs no computer, so it has no use for OpenShell.
playbooks=("$dir/ansible/site.yml" "$dir/ansible/demo.yml")
[ "$ROLE" != orchestrator ] || playbooks=("$dir/ansible/demo.yml")
shared=()
[ "$ROLE" = all ] || shared=(-e "demo_efs=$EFS.efs.$REGION.amazonaws.com" -e "demo_run=$RUN")

ansible-playbook -c local -i 'spike,' \
  -e "@$dir/demo-vars.json" -e "demo_bucket=$BUCKET" -e "demo_region=$REGION" -e "demo_image_key=$(cat "$dir/image.key")" \
  -e "demo_bake=$([ "$BAKE" = on ] && echo true || echo false)" -e "demo_lease_minutes=$LEASE_MINUTES" -e "demo_max_hours=$MAX_HOURS" -e demo_check_public=false -e "demo_role=$ROLE" -e "demo_live=$([ "$LIVE" = on ] && echo true || echo false)" -e "demo_release=$RELEASE" ${shared[@]+"${shared[@]}"} \
  "${playbooks[@]}"
if [ "$BAKE" = on ]; then
  # The gateway turns the runner image into the disk a sandbox starts from the first time it is asked for one, which is most of a
  # cold start. It is asked here, as the cell asks it, and the disk is kept in the image.
  systemd-run --wait --collect --unit=haas-bake-survey -p User=ubuntu -p SupplementaryGroups=kvm -p WorkingDirectory=/var/lib/haas \
    -p EnvironmentFile=/etc/haas/org.env -p EnvironmentFile=/etc/haas/cell.env -p EnvironmentFile=/run/haas/cell.env /usr/local/bin/haas survey
  touch "${HAAS_BAKE_DONE:-/etc/haas/bake-done}"
fi
echo "haas-bootstrap: done"
