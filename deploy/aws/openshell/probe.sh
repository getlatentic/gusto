#!/usr/bin/env bash
# What a sandbox is and is not allowed, measured from inside one. The output is
# kept in out/probe.txt so a run can be compared with the last.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
{
  echo "OpenShell probe, $(date -u +%FT%TZ), $(terraform output -raw instance_id)"
  ssh "${ssh_options[@]}" "ubuntu@$address" 'bash -s' < probe/on-host.sh
} | tee out/probe.txt
