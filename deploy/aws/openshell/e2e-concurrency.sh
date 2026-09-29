#!/usr/bin/env bash
# How many sandboxes one gateway runs at once, on this instance: see e2e/concurrency-on-host.sh.
# It removes the sandbox and the forward that e2e-shared.sh left. Output is kept in out/e2e-concurrency.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e/shared'
scp -q "${ssh_options[@]}" e2e/concurrency-on-host.sh "ubuntu@$address:e2e/shared/"
ssh "${ssh_options[@]}" "ubuntu@$address" "SIZES='${SIZES:-1 2 4 8 12}' WAVE='${WAVE:-4}' bash ~/e2e/shared/concurrency-on-host.sh" 2>&1 | tee out/e2e-concurrency.txt
