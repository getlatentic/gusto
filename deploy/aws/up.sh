#!/usr/bin/env bash
# Creates the POC and waits until E2B answers. About $0.42 an hour while it
# runs; down.sh removes everything.
source "$(dirname "$0")/env.sh"
terraform init -input=false -upgrade >/dev/null
# In an account that has never had an Auto Scaling group, AWS makes the group's
# service-linked role during this apply, and the first launch can fail with
# "Authentication Failure" before the role takes effect. Once is enough.
log="$(mktemp)"
if ! terraform apply -input=false -auto-approve 2>&1 | tee "$log"; then
  grep -q 'Authentication Failure' "$log" || exit 1
  echo "the new Auto Scaling role was not ready; applying again in a minute"
  sleep 60
  terraform apply -input=false -auto-approve
fi
api="$(terraform output -raw e2b_api_url)"
printf 'waiting for E2B at %s ' "$api"
for _ in $(seq 1 90); do
  if curl -fsS -o /dev/null --max-time 5 "$api/health"; then
    echo; echo "up: $(terraform output -raw hostname) -> $(terraform output -raw address)"
    echo "the first sandbox can start about a minute after this, once the base template is built"
    exit 0
  fi
  printf '.'; sleep 10
done
echo; echo "E2B did not answer in 15 minutes; check.sh shows the instance's own view" >&2
exit 1
