#!/usr/bin/env bash
# Sets up seat billing in a Stripe account and stores what a front door bills with. It makes the price of a seat, a month at
# GUSTO_CLOUD_SEAT_USD dollars (25), and the webhook at https://<cloud domain>/stripe/webhook, or finds the ones it made before.
# The key is read at a hidden prompt and passed to curl on its standard input, so it is on no command line and in no history.
# A test key is the default; a live key is taken only with --live. The key and the webhook's signing secret are kept as
# SecureStrings, the price id as a String, and a front door reads all three at its next boot.
source "$(dirname "$0")/env.sh"

live=no
[ "${1:-}" = --live ] && live=yes
seat_usd=${GUSTO_CLOUD_SEAT_USD:-25}
lookup=gusto-cloud-seat-monthly
# STRIPE_API is where Stripe is; a test points it at a fake.
api_base=${STRIPE_API:-https://api.stripe.com}
events=(customer.subscription.created customer.subscription.updated customer.subscription.deleted customer.subscription.paused customer.subscription.resumed)

domain=$(aws ssm get-parameter --name /gusto/cloud-config --query Parameter.Value --output text | python3 -c 'import json,sys; print(json.load(sys.stdin)["cloud_domain"])')
hook_url="https://$domain/stripe/webhook"

read -r -s -p "The Stripe secret key (sk_... or rk_...): " key
echo
case "$key" in
sk_test_* | rk_test_*) mode="test" ;;
sk_live_* | rk_live_*)
  [ "$live" = yes ] || { echo "that is a live key: run with --live to bill real cards" >&2; exit 2; }
  mode="live" ;;
*) echo "that is not a Stripe secret key" >&2; exit 2 ;;
esac

# api calls Stripe with the key on curl's standard input, and prints the answer, or Stripe's reason on standard error. The rest
# are curl's arguments: -d name=value for a field, and -G to send the fields as a query.
api() {
  local method=$1 path=$2 answer
  shift 2
  if ! answer=$(printf 'user = "%s:"\n' "$key" | curl -sS --fail-with-body --max-time 30 -K - -X "$method" "$api_base$path" "$@"); then
    echo "Stripe refused $method $path: $answer" >&2
    return 1
  fi
  printf '%s' "$answer"
}
# field prints one field of the JSON on standard input, by a Python expression over it called j.
field() { python3 -c "import json,sys; j=json.load(sys.stdin); print($1)"; }

dashboard=https://dashboard.stripe.com
[ "$mode" = test ] && dashboard=$dashboard/test

# The portal is where an admin changes the card and ends the plan, and gusto cloud opens it with the account's default settings.
defaults=$(api GET /v1/billing_portal/configurations -G -d is_default=true -d active=true | field 'len(j["data"])')
if [ "$defaults" = 0 ]; then
  echo "the customer portal has no settings yet: press Save once at $dashboard/settings/billing/portal, then run this again" >&2
  exit 1
fi

prices=$(api GET /v1/prices -G -d "lookup_keys[]=$lookup" -d active=true)
found=$(field 'len(j["data"])' <<<"$prices")
if [ "$found" = 0 ]; then
  product=$(api POST /v1/products -d "name=gusto cloud seat" -d "description=One person in a gusto cloud organization, a month" | field 'j["id"]')
  price=$(api POST /v1/prices -d "product=$product" -d currency=usd -d "unit_amount=$((seat_usd * 100))" \
    -d "recurring[interval]=month" -d "recurring[usage_type]=licensed" -d "lookup_key=$lookup" | field 'j["id"]')
  echo "made the price $price: $seat_usd dollars a seat a month"
else
  price=$(field 'j["data"][0]["id"]' <<<"$prices")
  amount=$(field 'j["data"][0]["unit_amount"]' <<<"$prices")
  [ "$amount" = $((seat_usd * 100)) ] || { echo "the price $price is $amount cents, not $seat_usd dollars: archive it in $dashboard/products and run this again" >&2; exit 1; }
  echo "found the price $price: $seat_usd dollars a seat a month"
fi

# Stripe shows a webhook's signing secret once, when the webhook is made. One already made is kept only when the secret stored
# is for the same mode; otherwise it is removed in the dashboard and made again here.
hooks=$(api GET /v1/webhook_endpoints -G -d limit=100)
hook=$(python3 -c 'import json,sys; print(next((h["id"] for h in json.load(sys.stdin)["data"] if h["url"] == sys.argv[1]), ""))' "$hook_url" <<<"$hooks")
enabled=()
for e in "${events[@]}"; do enabled+=(-d "enabled_events[]=$e"); done
if [ -z "$hook" ]; then
  made=$(api POST /v1/webhook_endpoints --data-urlencode "url=$hook_url" -d "description=gusto cloud seat billing" "${enabled[@]}")
  hook=$(field 'j["id"]' <<<"$made")
  webhook=$(field 'j["secret"]' <<<"$made")
  echo "made the webhook $hook to $hook_url"
else
  stored_key=$(aws ssm get-parameter --name /gusto/stripe-key --with-decryption --query Parameter.Value --output text 2>/dev/null || true)
  webhook=$(aws ssm get-parameter --name /gusto/stripe-webhook-secret --with-decryption --query Parameter.Value --output text 2>/dev/null || true)
  case "$stored_key" in sk_"$mode"_* | rk_"$mode"_*) ;; *) webhook= ;; esac
  unset stored_key
  [ -n "$webhook" ] || { echo "the webhook $hook to $hook_url has a secret this machine does not have: delete it at $dashboard/webhooks and run this again" >&2; exit 1; }
  api POST "/v1/webhook_endpoints/$hook" "${enabled[@]}" >/dev/null
  echo "found the webhook $hook to $hook_url"
fi

aws ssm put-parameter --name /gusto/stripe-key --type SecureString --overwrite --value "$key" --query Version --output text >/dev/null
aws ssm put-parameter --name /gusto/stripe-webhook-secret --type SecureString --overwrite --value "$webhook" --query Version --output text >/dev/null
aws ssm put-parameter --name /gusto/stripe-price --type String --overwrite --value "$price" --query Version --output text >/dev/null
unset key webhook
echo "stored /gusto/stripe-key, /gusto/stripe-webhook-secret and /gusto/stripe-price ($mode mode) in $AWS_REGION; a front door bills with them from its next boot"
