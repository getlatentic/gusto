#!/bin/bash
# Runs once, as root, when a host is launched, and makes it a host of the public
# cloud from the release bundle in the store bucket: no person and no SSH is needed. The launcher
# fills in the settings on the next lines. ROLE is all (a host that is the front door and runs the
# organizations' servers), orchestrator (the front door alone) or cell (runs servers for a front door);
# the last two share the file system EFS, and RUN is the directory of it that this cloud's hosts use.
# LIVE is on for the gusto cloud that stays up: it has no lease and no limit on how long it runs, and is brought up to a newer
# release by running this again (GUSTO_LIVE_ENV names the file that keeps what it was started with).
# SERVERS=shared runs every organization's server in one process, SANDBOX names the one kind of sandbox the host's agents run in
# (container needs no OpenShell), COMPUTER_MEMORY is what each may use, and DATABASE=postgres keeps the accounts in Postgres on the
# host. Each is kept for upgrades, as LIVE's settings are.
set -Eeuo pipefail
RELEASE=${RELEASE:-@RELEASE@} BUCKET=${BUCKET:-@BUCKET@} REGION=${REGION:-@REGION@}
LEASE_MINUTES=${LEASE_MINUTES:-@LEASE_MINUTES@} MAX_HOURS=${MAX_HOURS:-@MAX_HOURS@}
ROLE=${ROLE:-@ROLE@} RUN=${RUN:-@RUN@} EFS=${EFS:-@EFS@} LIVE=${LIVE:-@LIVE@}
SERVERS=${SERVERS:-on} SANDBOX=${SANDBOX:-} COMPUTER_MEMORY=${COMPUTER_MEMORY:-} DATABASE=${DATABASE:-sqlite}
live_env=${GUSTO_LIVE_ENV:-/etc/gusto/live.env}
dir=${GUSTO_BUNDLE_DIR:-/opt/gusto-bundle} tmp=${GUSTO_TMP:-/tmp}
exec > >(tee -a "${GUSTO_BOOTSTRAP_LOG:-/var/log/gusto-bootstrap.log}") 2>&1

# A host that never finishes starting is still removed: the instance terminates when it powers off. One that
# is meant to stay up is not, since it is looked at and started again rather than lost.
if [ "$LIVE" != on ]; then
  systemd-run --on-active="${MAX_HOURS}h" --unit=gusto-deadman systemctl poweroff
  trap 'echo "gusto-bootstrap: failed, so the host powers off in 15 minutes"; systemd-run --on-active=15m --unit=gusto-failed systemctl poweroff' ERR
fi

# A small host needs room for the installs of a start and of an upgrade, which the memory it runs in does not leave.
if [ ! -e "${GUSTO_SWAPFILE:-/swapfile}" ] && [ "$(awk '/MemTotal/ {print int($2/1024)}' "${GUSTO_MEMINFO:-/proc/meminfo}")" -lt 3000 ]; then
  swapfile=${GUSTO_SWAPFILE:-/swapfile}
  fallocate -l 1G "$swapfile" && chmod 600 "$swapfile" && mkswap "$swapfile" >/dev/null && swapon "$swapfile"
  grep -q "^$swapfile " "${GUSTO_FSTAB:-/etc/fstab}" || echo "$swapfile none swap sw 0 0" >> "${GUSTO_FSTAB:-/etc/fstab}"
fi

export HOME=${HOME:-/root}
export AWS_REGION=$REGION AWS_DEFAULT_REGION=$REGION PATH=$PATH:/snap/bin:/usr/local/bin
export DEBIAN_FRONTEND=noninteractive
# AWS's own installer is used when snap cannot download the AWS command, which it sometimes cannot. The steps are joined with &&
# because set -e does not hold in a function run after ||.
install_aws() {
  local status=0
  curl -fsSL --retry 3 "https://awscli.amazonaws.com/awscli-exe-linux-$(uname -m).zip" -o "$tmp/awscli.zip" \
    && unzip -qo "$tmp/awscli.zip" -d "$tmp" && "$tmp/aws/install" --update || status=$?
  rm -rf "$tmp/awscli.zip" "$tmp/aws"
  return "$status"
}

apt-get update -qq
apt-get install -y -qq ansible unzip curl ca-certificates
command -v aws >/dev/null || snap install aws-cli --classic || install_aws

# The newest release is the one CI last made, for a host that is started by hand.
[ "$RELEASE" != latest ] || RELEASE=$(aws s3 cp "s3://$BUCKET/releases/latest" -)

mkdir -p "$dir"
# Each AWS command takes seconds to start on a cold disk, so the three are run at once.
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz" "$tmp/bundle.tgz" --only-show-errors & bundle=$!
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz.sha256" "$tmp/bundle.tgz.sha256" --only-show-errors & sum=$!
# The AWS command, a snap, writes nothing to a file it is redirected to when it is run over SSM, so it is read
# into a variable.
cloud_vars=$(aws ssm get-parameter --name "${CONFIG:-/gusto/cloud-config}" --query Parameter.Value --output text)
wait "$bundle"; wait "$sum"
# Nothing of the bundle is run until it is the one CI made.
(cd "$tmp" && sha256sum -c bundle.tgz.sha256)
tar -xzf "$tmp/bundle.tgz" -C "$dir"
printf '%s\n' "$cloud_vars" > "$dir/cloud-vars.json"

if [ "$LIVE" = on ]; then
  mkdir -p "$(dirname "$live_env")"
  printf 'BUCKET=%s\nREGION=%s\nROLE=%s\nRUN=%s\nEFS=%s\nLIVE=on\nLEASE_MINUTES=%s\nMAX_HOURS=%s\nSERVERS=%s\nSANDBOX=%s\nCOMPUTER_MEMORY=%s\nDATABASE=%s\n' \
    "$BUCKET" "$REGION" "$ROLE" "$RUN" "$EFS" "$LEASE_MINUTES" "$MAX_HOURS" "$SERVERS" "$SANDBOX" "$COMPUTER_MEMORY" "$DATABASE" > "$live_env"
fi

# A front door alone runs no computer, so it has no use for OpenShell, and nor has a host whose agents run in containers.
playbooks=("$dir/ansible/site.yml" "$dir/ansible/cloud.yml")
{ [ "$ROLE" = orchestrator ] || [ "$SANDBOX" = container ]; } && playbooks=("$dir/ansible/cloud.yml")
# An Arm host runs the bundle's Arm programs and the runner image built for Arm, which is kept under its key with -arm64 after it.
image_key=$(cat "$dir/image.key") programs=$dir/.state
if [ "${GUSTO_MACHINE:-$(uname -m)}" = aarch64 ]; then image_key=$image_key-arm64 programs=$dir/.state/arm64; fi
chosen=(-e "cloud_servers=$SERVERS" -e "cloud_programs=$programs" -e "cloud_postgres=$([ "$DATABASE" = postgres ] && echo true || echo false)")
[ -z "$SANDBOX" ] || chosen+=(-e "public_sandbox=$SANDBOX" -e "cloud_demo_sandbox=$SANDBOX")
[ -z "$COMPUTER_MEMORY" ] || chosen+=(-e "cloud_computer_memory=$COMPUTER_MEMORY")
shared=()
[ "$ROLE" = all ] || shared=(-e "cloud_efs=$EFS.efs.$REGION.amazonaws.com" -e "cloud_run=$RUN")

ansible-playbook -c local -i 'host,' \
  -e "@$dir/cloud-vars.json" -e "cloud_bucket=$BUCKET" -e "cloud_region=$REGION" -e "cloud_image_key=$image_key" \
  "${chosen[@]}" -e "cloud_lease_minutes=$LEASE_MINUTES" -e "cloud_max_hours=$MAX_HOURS" -e cloud_check_public=false -e "cloud_role=$ROLE" -e "cloud_live=$([ "$LIVE" = on ] && echo true || echo false)" ${shared[@]+"${shared[@]}"} \
  "${playbooks[@]}"
echo "gusto-bootstrap: done"
