#!/usr/bin/env bash
# Checks that the live public cloud is ready to show: main is deployed, the pages a visitor
# opens answer, sign-in reaches the code page, certificates have time left, the hosts are up,
# no sign-in mail failed in a day, and the organization's last tasks passed. It reads and
# changes nothing, sends no mail and costs nothing.
#   ./demo-ready.sh [org]      # org defaults to getlatentic-com; exits 1 when a check fails
source "$(dirname "$0")/env.sh"
site="${DEMO_SITE:-gusto.getlatentic.com}"
org="${1:-getlatentic-com}"
front="https://$site" mine="https://$org.$site"
failed=0

ok() { printf '  ok    %s\n' "$1"; }
bad() { printf '  FAIL  %s\n' "$1"; failed=1; }
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }

# status prints the status code and the redirect of one request, which never follows it.
status() { curl -s -o /dev/null -m 15 -w '%{http_code} %{redirect_url}' "$@"; }
page() { curl -s -m 15 "$@"; }

# days_left is how many days the certificate a host serves is still valid.
days_left() {
  local end
  end="$(echo | openssl s_client -connect "$1:443" -servername "$1" 2>/dev/null | openssl x509 -noout -enddate | cut -d= -f2)"
  [ -n "$end" ] || { echo 0; return; }
  echo $(( ($(date -j -f '%b %e %T %Y %Z' "$end" +%s 2>/dev/null || date -d "$end" +%s) - $(date +%s)) / 86400 ))
}

# on_hosts runs a script on every running instance over SSM and prints each one's output.
on_hosts() {
  local ids command
  ids="$(aws ec2 describe-instances --filters Name=instance-state-name,Values=running --query 'Reservations[].Instances[].InstanceId' --output text)"
  [ -n "$ids" ] || return 1
  command="$(aws ssm send-command --instance-ids $ids --document-name AWS-RunShellScript --timeout-seconds 60 \
    --parameters "commands=[\"echo $(printf '%s' "$1" | base64 | tr -d '\n') | base64 -d | bash\"]" --query Command.CommandId --output text)"
  for id in $ids; do
    aws ssm wait command-executed --command-id "$command" --instance-id "$id" 2>/dev/null || true
    aws ssm get-command-invocation --command-id "$command" --instance-id "$id" --query StandardOutputContent --output text
  done
}

echo "release"
head="$(git ls-remote https://github.com/getlatentic/gusto.git refs/heads/main | cut -c1-40)"
for workflow in deploy "demo bundle"; do
  run="$(gh run list -R getlatentic/gusto --workflow "$workflow" --commit "$head" --limit 1 --json conclusion --jq '.[0].conclusion // "none"')"
  check "$workflow of main ${head:0:8}: $run" '[ "$run" = success ]'
done

echo "pages at $front"
landing="$(page "$front/")"
check "the landing page offers Sign in" 'grep -q "href=\"/signin\"" <<<"$landing"'
check "the demo page answers" '[ "$(status "$front/demo" | cut -d" " -f1)" = 200 ]'
check "the sign-up page answers" '[ "$(status "$front/signup" | cut -d" " -f1)" = 200 ]'
check "/app sends a stranger to sign in" '[ "$(status "$front/app")" = "303 $front/signin" ]'
# An address under example.com has no account, so the page answers as for anybody and no mail is sent.
sent="$(page -d email=readiness@example.com "$front/signin")"
check "signing in leads to the code page" 'grep -q "/signin/code" <<<"$sent"'

echo "$org"
check "it is ready" '[ "$(page "$mine/readyz")" = ready ]'
check "a stranger is sent to the front door" '[ "$(status "$mine/login" | cut -d" " -f2)" = "$front/open?org=$org" ]'
check "its connector client document answers" '[ "$(status "$mine/connectors/client.json" | cut -d" " -f1)" = 200 ]'

echo "certificates"
for host in "$site" "$org.$site"; do
  left="$(days_left "$host")"
  check "$host: $left days left" '[ "$left" -ge 14 ]'
done

echo "hosts"
report="$(on_hosts "
if systemctl is-active -q gusto-cloud; then
  echo front-door
  echo mail-failures \$(journalctl -u gusto-cloud --since -24h -q | grep -c 'mail to a person was not sent')
fi
systemctl is-active -q gusto-cell && echo cell
jobs=/var/lib/gusto-cloud/cloud/orgs/$org/data/jobs
if [ -d \$jobs ] && systemctl is-active -q gusto-cell; then
  for dir in \$(ls \$jobs | sort | tail -5); do
    echo task \$dir \$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get(\"state\"))' \$jobs/\$dir/state.json)
  done
fi
" || true)"
check "the front door is running" 'grep -q "^front-door$" <<<"$report"'
check "a cell is running" 'grep -q "^cell$" <<<"$report"'
mail="$(grep '^mail-failures' <<<"$report" | cut -d' ' -f2 || true)"
check "no sign-in mail failed in 24 hours (${mail:-unknown})" '[ "${mail:-x}" = 0 ]'
# Only the newest task decides: an older failure was already followed by a task that ran.
tasks="$(grep '^task ' <<<"$report" | sort -u | cut -d' ' -f2- || true)"
newest="$(tail -1 <<<"$tasks" | cut -d' ' -f1)"
while read -r name state; do
  [ -n "$name" ] || continue
  if [ "$name" = "$newest" ]; then
    check "newest task $name: $state" '[ "$state" != failed ]'
  elif [ "$state" = failed ]; then
    printf '  note  task %s failed; a later one ran\n' "$name"
  fi
done <<<"$tasks"
[ -n "$tasks" ] || bad "no tasks of $org were found"

echo
if [ "$failed" = 0 ]; then echo "ready"; else echo "not ready"; exit 1; fi
