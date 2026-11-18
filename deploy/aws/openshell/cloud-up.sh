#!/usr/bin/env bash
# Brings up a host that serves the public cloud at https://$GUSTO_CLOUD_DOMAIN, organizations at
# https://$GUSTO_CLOUD_DOMAIN/workspace/<slug> and the apps people share at https://<slug>--<app>.$GUSTO_CLOUD_APPS_DOMAIN ($GUSTO_CLOUD_DOMAIN when unset). Read cloud.env (see cloud.env.example), and run ./put-key.sh for
# both model keys and ./put-cloudflare.sh for both Cloudflare tokens first.
#
# The first run builds the runner image on the host, about ten minutes, and keeps it in the
# store bucket. Later runs download it. The certificate is kept there the same way.
# ONLY_CONFIGURE=1 skips the host and the image and applies the configuration again to the host
# that is up. INSTANCE_TYPE changes the size (m8i.2xlarge by default, $0.42 an hour), and ./cloud-down.sh
# removes the host.
source "$(dirname "$0")/env.sh"
[ -f cloud.env ] || { echo "no cloud.env: copy cloud.env.example and fill it in" >&2; exit 2; }
set -a; source cloud.env; set +a
for need in GUSTO_CLOUD_DOMAIN GUSTO_CLOUD_ZONE GUSTO_CLOUDFLARE_ACCOUNT_ID GUSTO_MAIL_FROM GUSTO_CERT_EMAIL; do
  [ -n "${!need:-}" ] || { echo "$need is not set in cloud.env" >&2; exit 2; }
done
for parameter in cloudflare-dns-token cloudflare-mail-token model-key deepseek-key; do
  aws ssm get-parameter --name "/gusto/poc/$parameter" --query Parameter.Name --output text >/dev/null 2>&1 \
    || { echo "/gusto/poc/$parameter is not stored: run ./put-key.sh or ./put-cloudflare.sh" >&2; exit 2; }
done

bucket="$(./cloud-store.sh)"
[[ "$bucket" =~ ^gusto-poc-demo-[0-9]{12}$ ]] || { echo "cloud-store.sh did not give a bucket name: $bucket" >&2; exit 1; }
root="$(git rev-parse --show-toplevel)"
harnesses="$(./harnesses.sh)"
image_key="$(./image-key.sh)"

"$root/web/build.sh"
(cd "$root" && for program in gusto:./cmd/gusto gusto-dns:./deploy/aws/openshell/host/dns; do
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/${program%%:*}" "${program#*:}"
done)

started="$(date -u +%H:%M:%S)"
if [ -z "${ONLY_CONFIGURE:-}" ]; then
  export TF_VAR_instance_type="${INSTANCE_TYPE:-m8i.2xlarge}" TF_VAR_public_https=true TF_VAR_cloud_bucket="$bucket" TF_VAR_self_terminate=true
  ./up.sh
  address="$(terraform output -raw address)"
  if ! aws s3api head-object --bucket "$bucket" --key "image/$image_key.tar.gz" >/dev/null 2>&1; then
    ./image.sh runner-all-rootfs "$harnesses"
    ssh "${ssh_options[@]}" "ubuntu@$address" "AWS_REGION=$AWS_REGION PATH=/snap/bin:/usr/local/bin:\$PATH aws s3 cp ~/e2e/runner-all-rootfs.tar.gz s3://$bucket/image/$image_key.tar.gz --only-show-errors"
  fi
fi

mail="cloudflare://$GUSTO_CLOUDFLARE_ACCOUNT_ID?from=$(python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$GUSTO_MAIL_FROM")"
ansible-playbook -i "$state/inventory.ini" ansible/cloud.yml \
  -e cloud_domain="$GUSTO_CLOUD_DOMAIN" -e cloud_zone="$GUSTO_CLOUD_ZONE" -e cloud_apps_domain="${GUSTO_CLOUD_APPS_DOMAIN:-}" -e cloud_apps_zone="${GUSTO_CLOUD_APPS_ZONE:-}" \
  -e cloud_bucket="$bucket" -e cloud_image_key="$image_key" \
  -e cloud_mail="$mail" -e cloud_cert_email="$GUSTO_CERT_EMAIL" -e cloud_free_mail="${GUSTO_CLOUD_FREE_MAIL:-allow}" \
  -e cloud_region="$AWS_REGION" -e cloud_models="${GUSTO_MODELS:-gemma/google.gemma-4-26b-a4b,gemma/google.gemma-4-31b,deepseek/deepseek-flash,meta/muse-spark-1.3}" \
  -e cloud_computer_memory="${GUSTO_COMPUTER_MEMORY:-3Gi}" \
  -e cloud_lease_minutes="${GUSTO_HOST_LEASE_MINUTES:-30}" -e cloud_max_hours="${GUSTO_HOST_MAX_HOURS:-4}"
echo "configured at $started UTC: https://$GUSTO_CLOUD_DOMAIN. The host removes itself ${GUSTO_HOST_LEASE_MINUTES:-30} minutes after it started unless it is extended at https://$GUSTO_CLOUD_DOMAIN/lease, and never runs past ${GUSTO_HOST_MAX_HOURS:-4} hours. ./cloud-down.sh removes it now."
