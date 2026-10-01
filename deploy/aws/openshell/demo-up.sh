#!/usr/bin/env bash
# Brings up a host that serves the public cloud at https://$DEMO_NAME, organisations at
# https://<slug>.$DEMO_NAME. Read demo.env (see demo.env.example), and run ./put-key.sh for
# both model keys and ./put-cloudflare.sh for both Cloudflare tokens first.
#
# The first run builds the runner image on the host, about ten minutes, and keeps it in the
# store bucket. Later runs download it. The certificate is kept there the same way.
# ONLY_CONFIGURE=1 skips the host and the image and applies the configuration again to the host
# that is up. INSTANCE_TYPE changes the size (m8i.2xlarge by default, $0.42 an hour), and ./demo-down.sh
# removes the host.
source "$(dirname "$0")/env.sh"
[ -f demo.env ] || { echo "no demo.env: copy demo.env.example and fill it in" >&2; exit 2; }
set -a; source demo.env; set +a
for need in DEMO_NAME DEMO_ZONE DEMO_ACCOUNT_ID DEMO_MAIL_FROM DEMO_CERT_EMAIL; do
  [ -n "${!need:-}" ] || { echo "$need is not set in demo.env" >&2; exit 2; }
done
for parameter in cloudflare-dns-token cloudflare-mail-token model-key deepseek-key; do
  aws ssm get-parameter --name "/haas/poc/$parameter" --query Parameter.Name --output text >/dev/null 2>&1 \
    || { echo "/haas/poc/$parameter is not stored: run ./put-key.sh or ./put-cloudflare.sh" >&2; exit 2; }
done

bucket="$(./demo-store.sh)"
[[ "$bucket" =~ ^haas-poc-demo-[0-9]{12}$ ]] || { echo "demo-store.sh did not give a bucket name: $bucket" >&2; exit 1; }
root="$(git rev-parse --show-toplevel)"
harnesses="$(./harnesses.sh)"
image_key="$(./image-key.sh)"

(cd "$root" && for program in haas:./cmd/haas haas-dns:./deploy/aws/openshell/demo/dns; do
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/${program%%:*}" "${program#*:}"
done)

started="$(date -u +%H:%M:%S)"
if [ -z "${ONLY_CONFIGURE:-}" ]; then
  export TF_VAR_instance_type="${INSTANCE_TYPE:-m8i.2xlarge}" TF_VAR_public_https=true TF_VAR_demo_bucket="$bucket" TF_VAR_self_terminate=true
  ./up.sh
  address="$(terraform output -raw address)"
  if ! aws s3api head-object --bucket "$bucket" --key "image/$image_key.tar.gz" >/dev/null 2>&1; then
    ./image.sh runner-all-rootfs "$harnesses"
    ssh "${ssh_options[@]}" "ubuntu@$address" "AWS_REGION=$AWS_REGION /snap/bin/aws s3 cp ~/e2e/runner-all-rootfs.tar.gz s3://$bucket/image/$image_key.tar.gz --only-show-errors"
  fi
fi

mail="cloudflare://$DEMO_ACCOUNT_ID?from=$(python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$DEMO_MAIL_FROM")"
ansible-playbook -i "$state/inventory.ini" ansible/demo.yml \
  -e demo_name="$DEMO_NAME" -e demo_zone="$DEMO_ZONE" -e demo_bucket="$bucket" -e demo_image_key="$image_key" \
  -e demo_mail="$mail" -e demo_cert_email="$DEMO_CERT_EMAIL" -e demo_free_mail="${DEMO_FREE_MAIL:-allow}" \
  -e demo_region="$AWS_REGION" -e demo_models="${DEMO_MODELS:-platform/qwen.qwen3-vl-235b-a22b-instruct,oss/openai.gpt-oss-120b}" \
  -e demo_computer_memory="${DEMO_COMPUTER_MEMORY:-3Gi}" \
  -e demo_lease_minutes="${DEMO_LEASE_MINUTES:-30}" -e demo_max_hours="${DEMO_MAX_HOURS:-4}"
echo "configured at $started UTC: https://$DEMO_NAME. The host removes itself ${DEMO_LEASE_MINUTES:-30} minutes after it started unless it is extended at https://$DEMO_NAME/lease, and never runs past ${DEMO_MAX_HOURS:-4} hours. ./demo-down.sh removes it now."
