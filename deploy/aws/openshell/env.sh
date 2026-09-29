# Sourced by up.sh, probe.sh and down.sh: the AWS profile and this machine's
# address. Nothing secret is read.
set -euo pipefail
export TF_IN_AUTOMATION=1 TF_CLI_ARGS="-no-color"
cd "$(dirname "$0")"
export PATH="$HOME/.local/bin:/opt/homebrew/bin:$PATH"
export AWS_PROFILE="${AWS_PROFILE:-haas-poc}"
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="$AWS_REGION"
state=".state"
mkdir -p "$state" out
export TF_VAR_operator_cidrs="[\"$(curl -fsS https://checkip.amazonaws.com | tr -d '\n')/32\"]"
export TF_VAR_public_key="$(cat "$state/key.pub" 2>/dev/null || true)"
ssh_options=(-i "$state/key" -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="$state/known_hosts" -o LogLevel=ERROR -o ConnectTimeout=10)
