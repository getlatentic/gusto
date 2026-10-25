#!/bin/bash
# Runs once, as root, when a demo host is launched, and makes it a demo host of the public
# cloud from the release bundle in the store bucket: no person and no SSH is needed. The launcher
# fills in the settings on the next lines. ROLE is all (a host that is the front door and runs the
# organizations' servers), orchestrator (the front door alone) or cell (runs servers for a front door);
# the last two share the file system EFS, and RUN is the directory of it that this demo's hosts use.
# BAKE is on for the host an image of a cell is made from: it is made as a cell is, its runner image is prepared for the gateway,
# and it is not started as a cell, so that the image holds everything a cell has to wait for and nothing that is its own. A host made
# from that image has /etc/gusto/baked and goes straight to configuring itself.
# KIT is the SHA-256 of the kit a front door made for its cells on the shared file system, and BAKE_KEY the image it was made
# for: a cell made from that image unpacks the kit and starts, with no playbook and nothing fetched.
# LIVE is on for the front door that stays up: it has no lease and no limit on how long it runs, keeps what
# is on the file system, starts hosts for cells when organizations need one, and is brought up to a newer
# release by running this again (GUSTO_LIVE_ENV names the file that keeps what it was started with).
set -Eeuo pipefail
RELEASE=${RELEASE:-@RELEASE@} BUCKET=${BUCKET:-@BUCKET@} REGION=${REGION:-@REGION@}
LEASE_MINUTES=${LEASE_MINUTES:-@LEASE_MINUTES@} MAX_HOURS=${MAX_HOURS:-@MAX_HOURS@}
ROLE=${ROLE:-@ROLE@} RUN=${RUN:-@RUN@} EFS=${EFS:-@EFS@} LIVE=${LIVE:-@LIVE@} BAKE=${BAKE:-off}
KIT=${KIT:-@KIT@} BAKE_KEY=${BAKE_KEY:-@BAKE_KEY@}
# A host started by hand, and not by a front door, has no kit.
[[ $KIT != @* ]] || KIT=""
[[ $BAKE_KEY != @* ]] || BAKE_KEY=""
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
# idle runs a command at idle disk priority, so the start-up's own reads of the disk go first.
idle() {
  if command -v ionice >/dev/null; then ionice -c3 "$@"; else "$@"; fi
}
warm_the_disk() {
  idle python3 - <<'PY'
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
# A sandbox reads the whole runner image it is made from to start, about a gigabyte with the files beside it, which a host
# that has just started has nowhere but on the disk: the first sandbox of a kept host took 14.6 s against 7.9 s for the next.
# They are read into memory on every start, at idle priority.
warm_the_cache() {
  local lib=${GUSTO_WARM_DIR:-/var/lib/gusto} images=${GUSTO_WARM_IMAGES:-/var/lib/openshell-xdg/openshell/vm-driver/images}
  idle find "$lib/runner-all-rootfs.tar.gz" "$images/overlay-templates" "$images"/sandbox-bootstrap-* "$images/../host-runtime" \
    -type f -exec cat {} + > /dev/null
}
if [ -e "$baked_marker" ]; then
  warm_the_cache > /dev/null 2>&1 &
fi

# A kept host runs this on every start and its disk was read on the first, so it is read once for each instance.
this_instance() {
  local token
  token=$(curl -fsS -m 5 -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60') &&
    curl -fsS -m 5 -H "X-aws-ec2-metadata-token: $token" http://169.254.169.254/latest/meta-data/instance-id
}
warmed=${GUSTO_WARMED:-/var/lib/gusto}/.warmed-${GUSTO_INSTANCE:-$(this_instance 2>/dev/null || echo unknown)}
if [ -e "$baked_marker" ] && [ ! -e "$warmed" ]; then
  echo "gusto-bootstrap: warming the disk of the image in the background"
  { warm_the_disk && touch "$warmed"; } > /dev/null 2>&1 &
fi

efs_mount=${GUSTO_EFS_MOUNT:-/mnt/efs} state=${GUSTO_STATE:-/var/lib/gusto-cloud} root=${GUSTO_ROOT:-}

# The shared file system, and this demo's directory of it where the organizations' data is, mounted as the playbook mounts them.
# A mount target answers a little after it is made.
mount_the_run() {
  mkdir -p "$efs_mount" "$state"
  for attempt in $(seq 1 30); do
    mount -t nfs4 -o nfsvers=4.1,rsize=1048576,wsize=1048576,hard,timeo=600,retrans=2,noresvport "$EFS.efs.$REGION.amazonaws.com:/" "$efs_mount" && break
    [ "$attempt" -lt 30 ] || return 1
    sleep 2
  done
  mkdir -p "$efs_mount/runs/$RUN"
  chown ubuntu:ubuntu "$efs_mount/runs/$RUN" 2>/dev/null || true
  mount --bind "$efs_mount/runs/$RUN" "$state"
}

# A cell takes the kit only when its sum is the one its front door started it with, since every cell can write the file system.
configure_from_kit() {
  local kit="$tmp/cell-kit.tgz"
  cp "$state/kits/$KIT.tgz" "$kit" || return 1
  echo "$KIT  $kit" | sha256sum -c --quiet || return 1
  tar -xzf "$kit" -C "${root:-/}"
  systemctl daemon-reload
  systemctl enable gusto-lease.timer
  systemctl restart gusto-boot
  systemctl start gusto-lease.timer gusto-cell
}

if [ "$ROLE" = cell ] && [ -n "$KIT" ] && [ -e "$baked_marker" ] && [ "$(cat "$baked_marker")" = "$BAKE_KEY" ]; then
  mount_the_run
  if configure_from_kit; then
    echo "gusto-bootstrap: configured from the kit $KIT"
    echo "gusto-bootstrap: done"
    exit 0
  fi
  echo "gusto-bootstrap: the kit $KIT is not there or not the one asked for, so the playbook configures the host"
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
# Each AWS command takes seconds to start on a cold disk, so the three are run at once.
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz" "$tmp/bundle.tgz" --only-show-errors & bundle=$!
aws s3 cp "s3://$BUCKET/releases/$RELEASE/bundle.tgz.sha256" "$tmp/bundle.tgz.sha256" --only-show-errors & sum=$!
# The AWS command, a snap, writes nothing to a file it is redirected to when it is run over SSM, so it is read
# into a variable.
demo_vars=$(aws ssm get-parameter --name /gusto/poc/demo-config --query Parameter.Value --output text)
wait "$bundle"; wait "$sum"
# Nothing of the bundle is run until it is the one CI made.
(cd "$tmp" && sha256sum -c bundle.tgz.sha256)
tar -xzf "$tmp/bundle.tgz" -C "$dir"
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

# The front door that starts cells writes the files of its cells once, here, and starts each with the kit's sum.
if [ "$ROLE" = orchestrator ] && [ "$LIVE" = on ] && [ -f "$dir/bake.key" ]; then
  kit_root=$(mktemp -d "$tmp/cell-kit.XXXXXX")
  ansible-playbook -c local -i 'spike,' \
    -e "@$dir/demo-vars.json" -e "demo_bucket=$BUCKET" -e "demo_region=$REGION" -e "demo_image_key=$(cat "$dir/image.key")" \
    -e "demo_release=$RELEASE" -e "demo_root=$kit_root" ${shared[@]+"${shared[@]}"} "$dir/ansible/cell-kit.yml"
  kit_file=${GUSTO_KIT_FILE:-/usr/local/lib/gusto/cell-kit.tgz}
  mkdir -p "$(dirname "$kit_file")"
  (cd "$kit_root" && find . -type f -print0 | tar --null --owner=0 --group=0 -czf "$kit_file" -T -)
  rm -rf "$kit_root"
  shared+=(-e "demo_cell_kit=$(sha256sum "$kit_file" | cut -d' ' -f1)" -e "demo_cell_kit_file=$kit_file" -e "demo_bake_key=$(cat "$dir/bake.key")")
fi

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
