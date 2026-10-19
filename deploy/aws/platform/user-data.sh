#!/bin/bash
# Runs once, as root, when a demo host is launched, and makes it a demo host of the public
# cloud from the release bundle in the store bucket: no person and no SSH is needed. The launcher
# fills in the settings on the next lines. ROLE is all (a host that is the front door and runs the
# organisations' servers), orchestrator (the front door alone) or cell (runs servers for a front door);
# the last two share the file system EFS, and RUN is the directory of it that this demo's hosts use.
# BAKE is on for the host an image of a cell is made from: it is made as a cell is, its runner image is prepared for the gateway,
# and it is not started as a cell, so that the image holds everything a cell has to wait for and nothing that is its own. A host made
# from that image has /etc/gusto/baked and goes straight to configuring itself.
# LIVE is on for the front door that stays up: it has no lease and no limit on how long it runs, keeps what
# is on the file system, starts hosts for cells when organisations need one, and is brought up to a newer
# release by running this again (GUSTO_LIVE_ENV names the file that keeps what it was started with).
set -Eeuo pipefail
RELEASE=${RELEASE:-@RELEASE@} BUCKET=${BUCKET:-@BUCKET@} REGION=${REGION:-@REGION@}
LEASE_MINUTES=${LEASE_MINUTES:-@LEASE_MINUTES@} MAX_HOURS=${MAX_HOURS:-@MAX_HOURS@}
ROLE=${ROLE:-@ROLE@} RUN=${RUN:-@RUN@} EFS=${EFS:-@EFS@} LIVE=${LIVE:-@LIVE@} BAKE=${BAKE:-off}
baked_marker=${GUSTO_BAKED:-/etc/gusto/baked}
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
# A host made from the image of a cell gets the disk the image was taken from a block at a time, as each is first read, which is slow and
# was seen to cost a first sandbox two and a half minutes: the runner image is read whole to be named, and the disk a sandbox starts from
# is read to be copied. They are read here, in many streams, while the rest of the start-up goes on, so that the first sandbox finds them.
warm_the_disk() {
  python3 - <<'PY'
import concurrent.futures as futures, os, sys
CHUNK = 8 << 20
roots = [os.environ.get("GUSTO_WARM_DIR", "/var/lib/gusto"), os.environ.get("GUSTO_WARM_IMAGES", "/var/lib/openshell-xdg/openshell/vm-driver/images")]
spans = []
for root in roots:
    for dirpath, _, names in os.walk(root):
        for name in names:
            path = os.path.join(dirpath, name)
            try:
                fd = os.open(path, os.O_RDONLY)
                size, at = os.fstat(fd).st_size, 0
                while at < size:
                    try:
                        data = os.lseek(fd, at, os.SEEK_DATA)
                    except OSError:
                        break
                    hole = os.lseek(fd, data, os.SEEK_HOLE)
                    spans.append((path, data, hole))
                    at = hole
                os.close(fd)
            except OSError:
                pass
def read(span):
    path, start, end = span
    try:
        with open(path, "rb", buffering=0) as handle:
            for at in range(start, end, CHUNK):
                os.pread(handle.fileno(), min(CHUNK, end - at), at)
    except OSError:
        pass
with futures.ThreadPoolExecutor(max_workers=16) as pool:
    chunks = [(p, a, min(a + 64 * CHUNK, e)) for p, s, e in spans for a in range(s, e, 64 * CHUNK)]
    list(pool.map(read, chunks))
PY
}
if [ -e "$baked_marker" ]; then
  echo "gusto-bootstrap: warming the disk of the image in the background"
  warm_the_disk > /dev/null 2>&1 &
fi

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
demo_vars=$(aws ssm get-parameter --name /gusto/poc/demo-config --query Parameter.Value --output text)
printf '%s\n' "$demo_vars" > "$dir/demo-vars.json"

if [ "$LIVE" = on ]; then
  mkdir -p "$(dirname "$live_env")"
  printf 'BUCKET=%s\nREGION=%s\nROLE=%s\nRUN=%s\nEFS=%s\nLIVE=on\nLEASE_MINUTES=%s\nMAX_HOURS=%s\n' "$BUCKET" "$REGION" "$ROLE" "$RUN" "$EFS" "$LEASE_MINUTES" "$MAX_HOURS" > "$live_env"
fi

# A host made from the image of a cell goes straight to configuring itself when the release was made for that image, which is when its key
# is the one the image was made with. A release that expects another image gets everything installed, which is slower and right.
demo_baked=false
if [ -e "$baked_marker" ] && [ -f "$dir/bake.key" ] && [ "$(cat "$baked_marker")" = "$(cat "$dir/bake.key")" ]; then
  demo_baked=true
fi
# A front door alone runs no computer, so it has no use for OpenShell, and nor has a host that has it already.
playbooks=("$dir/ansible/site.yml" "$dir/ansible/demo.yml")
{ [ "$ROLE" = orchestrator ] || [ "$demo_baked" = true ]; } && playbooks=("$dir/ansible/demo.yml")
shared=()
[ "$ROLE" = all ] || shared=(-e "demo_efs=$EFS.efs.$REGION.amazonaws.com" -e "demo_run=$RUN")

ansible-playbook -c local -i 'spike,' \
  -e "@$dir/demo-vars.json" -e "demo_bucket=$BUCKET" -e "demo_region=$REGION" -e "demo_image_key=$(cat "$dir/image.key")" \
  -e "demo_baked=$demo_baked" -e "demo_bake=$([ "$BAKE" = on ] && echo true || echo false)" -e "demo_lease_minutes=$LEASE_MINUTES" -e "demo_max_hours=$MAX_HOURS" -e demo_check_public=false -e "demo_role=$ROLE" -e "demo_live=$([ "$LIVE" = on ] && echo true || echo false)" -e "demo_release=$RELEASE" ${shared[@]+"${shared[@]}"} \
  "${playbooks[@]}"
if [ "$BAKE" = on ]; then
  # The gateway turns the runner image into the disk a sandbox starts from the first time it is asked for one, which is most of a
  # cold start. It is asked here, as the cell asks it, and the disk is kept in the image.
  systemd-run --wait --collect --unit=gusto-bake-survey -p User=ubuntu -p SupplementaryGroups=kvm -p WorkingDirectory=/var/lib/gusto \
    -p EnvironmentFile=/etc/gusto/org.env -p EnvironmentFile=/etc/gusto/cell.env -p EnvironmentFile=/run/gusto/cell.env /usr/local/bin/gusto scan
  touch "${GUSTO_BAKE_DONE:-/etc/gusto/bake-done}"
fi
echo "gusto-bootstrap: done"
