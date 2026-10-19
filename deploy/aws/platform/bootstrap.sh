#!/usr/bin/env bash
# Makes the persistent AWS pieces of the public cloud demo, once, from a machine with the rights
# to make IAM roles: the role a host has, the launch template a host is made from, and the role GitHub
# Actions takes. Applying it again changes what changed. After it, hosts are made and removed by the
# workflows in .github/workflows and nothing runs from a laptop. The CI role can launch and remove
# hosts tagged project=haas-poc and put objects in the bucket, and has no IAM right, so it cannot
# widen itself: a change to this stack is made here, by a person.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH" TF_IN_AUTOMATION=1 TF_CLI_ARGS="-no-color"
export AWS_PROFILE="${AWS_PROFILE:-haas-poc}" AWS_REGION="${AWS_REGION:-us-east-1}" AWS_DEFAULT_REGION="${AWS_REGION:-us-east-1}"
# How GitHub names this repository in a workflow's token depends on the repository: the immutable form
# carries the ids of the owner and the repository. Asking GitHub is the only way to know.
subject="$(gh api repos/getlatentic/gusto/actions/oidc/customization/sub --jq .sub_claim_prefix 2>/dev/null || true)"
subject="${subject:-repo:getlatentic/gusto}"
echo "the workflows' token names the repository as $subject"
bucket="$(../openshell/demo-store.sh)"
terraform init -input=false -reconfigure -backend-config="bucket=$bucket" -backend-config="region=$AWS_REGION" >/dev/null
if ! terraform apply -input=false -auto-approve -var "bucket=$bucket" -var "region=$AWS_REGION" -var "manage_github_provider=${MANAGE_GITHUB_PROVIDER:-false}" -var "github_subject=$subject" -var "alert_email=${ALERT_EMAIL:-}"; then
  cat >&2 <<MSG

If the error is about GitHub's identity provider, this profile may not make one. Make it once, with an
identity that may (CloudShell in the console, or an admin profile), and run this again:

  aws iam create-open-id-connect-provider --url https://token.actions.githubusercontent.com --client-id-list sts.amazonaws.com

An admin profile can let this script make it instead: MANAGE_GITHUB_PROVIDER=true ./bootstrap.sh
MSG
  exit 1
fi
echo
echo "Set these once on the repository, and the workflows can run:"
echo "  gh variable set GUSTO_AWS_ROLE   --repo getlatentic/gusto --body '$(terraform output -raw ci_role)'"
echo "  gh variable set GUSTO_BUCKET     --repo getlatentic/gusto --body '$bucket'"
echo "  gh variable set GUSTO_DEMO_NAME  --repo getlatentic/gusto --body \"\$DEMO_NAME\"   # from demo.env"
echo "  gh variable set GUSTO_EFS        --repo getlatentic/gusto --body '$(terraform output -raw file_system)'   # only for demos with cells"
