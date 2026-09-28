# Sourced by up.sh, down.sh and check.sh: the AWS profile and the Cloudflare
# token, read from files outside the repository.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$HOME/.local/bin:$PATH"
export AWS_PROFILE="${AWS_PROFILE:-haas-poc}"
export AWS_REGION="${AWS_REGION:-us-east-1}"
token_file="${HAAS_CLOUDFLARE_TOKEN_FILE:-$HOME/.config/haas/cloudflare_token}"
[ -r "$token_file" ] || { echo "no Cloudflare token at $token_file" >&2; exit 1; }
CLOUDFLARE_API_TOKEN="$(tr -d '\n' < "$token_file")"
export CLOUDFLARE_API_TOKEN
# Terraform asks for these on destroy too, though destroy does not use them.
export TF_VAR_operator_cidrs="[\"$(curl -fsS https://checkip.amazonaws.com | tr -d '\n')/32\"]"
export TF_VAR_budget_email="${HAAS_BUDGET_EMAIL:-$(git config user.email)}"
