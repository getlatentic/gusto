// Package stripesetup tests put-stripe.sh, which sets up seat billing in a Stripe account, against a fake Stripe and a fake
// Parameter Store.
package stripesetup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

const testKey = "sk_test_abc"

// fakeAWS keeps parameters as files named for them, as `aws ssm get-parameter` and `put-parameter` would.
const fakeAWS = `#!/usr/bin/env bash
set -eu
cmd=$2; shift 2
name= value=
while [ $# -gt 0 ]; do
  case "$1" in --name) name=$2; shift ;; --value) value=$2; shift ;; esac
  shift
done
file="$FAKE_SSM/$(printf '%s' "$name" | tr / _)"
case "$cmd" in
get-parameter) [ -f "$file" ] || { echo "ParameterNotFound" >&2; exit 254; }; cat "$file"; echo ;;
put-parameter) printf '%s' "$value" > "$file"; echo 1 ;;
esac
`

// stripe is the part of Stripe's API put-stripe.sh calls.
type stripe struct {
	mu       sync.Mutex
	portal   bool
	prices   []map[string]any
	hooks    []map[string]any
	products int
	events   []string
}

func (s *stripe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if user, _, _ := r.BasicAuth(); user != testKey && user != "sk_live_abc" {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Invalid API Key provided"}}`)
		return
	}
	r.ParseForm()
	answer := func(v any) { json.NewEncoder(w).Encode(v) }
	list := func(data []map[string]any) { answer(map[string]any{"data": append([]map[string]any{}, data...)}) }
	switch r.Method + " " + r.URL.Path {
	case "GET /v1/billing_portal/configurations":
		if s.portal && r.Form.Get("is_default") == "true" {
			list([]map[string]any{{"id": "bpc_1"}})
		} else {
			list(nil)
		}
	case "GET /v1/prices":
		var found []map[string]any
		for _, p := range s.prices {
			if slices.Contains(r.Form["lookup_keys[]"], p["lookup_key"].(string)) {
				found = append(found, p)
			}
		}
		list(found)
	case "POST /v1/products":
		s.products++
		answer(map[string]any{"id": fmt.Sprintf("prod_%d", s.products)})
	case "POST /v1/prices":
		var cents int
		fmt.Sscan(r.Form.Get("unit_amount"), &cents)
		p := map[string]any{"id": fmt.Sprintf("price_%d", len(s.prices)+1), "unit_amount": cents, "lookup_key": r.Form.Get("lookup_key"),
			"product": r.Form.Get("product"), "currency": r.Form.Get("currency"), "interval": r.Form.Get("recurring[interval]")}
		s.prices = append(s.prices, p)
		answer(p)
	case "GET /v1/webhook_endpoints":
		list(s.hooks)
	case "POST /v1/webhook_endpoints":
		h := map[string]any{"id": fmt.Sprintf("we_%d", len(s.hooks)+1), "url": r.Form.Get("url")}
		s.hooks = append(s.hooks, h)
		s.events = r.Form["enabled_events[]"]
		answer(map[string]any{"id": h["id"], "url": h["url"], "secret": "whsec_made"})
	default:
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/webhook_endpoints/") {
			s.events = r.Form["enabled_events[]"]
			answer(map[string]any{"id": strings.TrimPrefix(r.URL.Path, "/v1/webhook_endpoints/")})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error":{"message":"no %s %s"}}`, r.Method, r.URL.Path)
	}
}

type setup struct {
	t      *testing.T
	dir    string
	ssm    string
	stripe *stripe
	api    string
}

func newSetup(t *testing.T) *setup {
	t.Helper()
	dir := t.TempDir()
	script, err := os.ReadFile("../put-stripe.sh")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	ssm := filepath.Join(dir, "ssm")
	os.MkdirAll(bin, 0o755)
	os.MkdirAll(ssm, 0o755)
	os.WriteFile(filepath.Join(dir, "put-stripe.sh"), script, 0o755)
	os.WriteFile(filepath.Join(dir, "env.sh"), []byte("set -euo pipefail\nexport AWS_REGION=us-east-1\n"), 0o644)
	os.WriteFile(filepath.Join(bin, "aws"), []byte(fakeAWS), 0o755)
	os.WriteFile(filepath.Join(ssm, "_gusto_cloud-config"), []byte(`{"cloud_domain": "gusto.example.com"}`), 0o644)
	s := &stripe{portal: true}
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	return &setup{t: t, dir: dir, ssm: ssm, stripe: s, api: server.URL}
}

// run runs the script with the key typed at its prompt, and says what it printed and whether it succeeded.
func (s *setup) run(key string, args ...string) (string, error) {
	cmd := exec.Command("bash", append([]string{filepath.Join(s.dir, "put-stripe.sh")}, args...)...)
	cmd.Stdin = strings.NewReader(key + "\n")
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(s.dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_SSM="+s.ssm, "STRIPE_API="+s.api)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (s *setup) stored(name string) string {
	b, err := os.ReadFile(filepath.Join(s.ssm, strings.ReplaceAll(name, "/", "_")))
	if err != nil {
		return ""
	}
	return string(b)
}

// A test account is given a price of 25 dollars a seat a month and a webhook to the cloud's own address for the
// subscription events, and the key, the price and the webhook's secret are stored. Run again, it makes nothing new.
func TestATestAccountIsSetUpOnceAndItsSecretsStored(t *testing.T) {
	s := newSetup(t)
	out, err := s.run(testKey)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(s.stripe.prices) != 1 || s.stripe.prices[0]["unit_amount"] != 2500 || s.stripe.prices[0]["currency"] != "usd" ||
		s.stripe.prices[0]["interval"] != "month" || s.stripe.products != 1 {
		t.Errorf("the seat's price: %v, %d products", s.stripe.prices, s.stripe.products)
	}
	want := []string{"customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted",
		"customer.subscription.paused", "customer.subscription.resumed"}
	if len(s.stripe.hooks) != 1 || s.stripe.hooks[0]["url"] != "https://gusto.example.com/stripe/webhook" || !slices.Equal(s.stripe.events, want) {
		t.Errorf("the webhook: %v %v", s.stripe.hooks, s.stripe.events)
	}
	if s.stored("/gusto/stripe-key") != testKey || s.stored("/gusto/stripe-price") != "price_1" || s.stored("/gusto/stripe-webhook-secret") != "whsec_made" {
		t.Errorf("stored %q %q %q", s.stored("/gusto/stripe-key"), s.stored("/gusto/stripe-price"), s.stored("/gusto/stripe-webhook-secret"))
	}
	if strings.Contains(out, testKey) || strings.Contains(out, "whsec_made") {
		t.Errorf("a secret was printed:\n%s", out)
	}

	out, err = s.run(testKey)
	if err != nil || len(s.stripe.prices) != 1 || len(s.stripe.hooks) != 1 || s.stored("/gusto/stripe-webhook-secret") != "whsec_made" {
		t.Errorf("run again: %v, %d prices, %d webhooks\n%s", err, len(s.stripe.prices), len(s.stripe.hooks), out)
	}
}

// A live key needs --live, and anything that is not a secret key is refused, before Stripe is called or anything stored.
func TestALiveKeyNeedsToBeAskedFor(t *testing.T) {
	s := newSetup(t)
	for _, key := range []string{"sk_live_abc", "pk_test_abc", ""} {
		if out, err := s.run(key); err == nil || len(s.stripe.prices) != 0 || s.stored("/gusto/stripe-key") != "" {
			t.Errorf("%q was taken: %v\n%s", key, err, out)
		}
	}
	if out, err := s.run("sk_live_abc", "--live"); err != nil || s.stored("/gusto/stripe-key") != "sk_live_abc" {
		t.Errorf("a live key with --live: %v\n%s", err, out)
	}
}

// Without portal settings an admin could not change their card, so nothing is made until they are saved.
func TestAnAccountWithNoPortalSettingsIsSentToSaveThem(t *testing.T) {
	s := newSetup(t)
	s.stripe.portal = false
	out, err := s.run(testKey)
	if err == nil || !strings.Contains(out, "https://dashboard.stripe.com/test/settings/billing/portal") || len(s.stripe.prices) != 0 ||
		s.stored("/gusto/stripe-key") != "" {
		t.Errorf("%v\n%s", err, out)
	}
}

// A webhook made before whose secret is not stored for this mode cannot be used, since Stripe shows the secret only once.
func TestAWebhookWhoseSecretIsLostIsNotUsed(t *testing.T) {
	s := newSetup(t)
	s.stripe.hooks = []map[string]any{{"id": "we_old", "url": "https://gusto.example.com/stripe/webhook"}}
	out, err := s.run(testKey)
	if err == nil || !strings.Contains(out, "delete it at https://dashboard.stripe.com/test/webhooks") || s.stored("/gusto/stripe-key") != "" {
		t.Errorf("%v\n%s", err, out)
	}
}

// A refusal from Stripe is shown with its reason.
func TestStripesReasonIsShown(t *testing.T) {
	s := newSetup(t)
	out, err := s.run("sk_test_wrong")
	if err == nil || !strings.Contains(out, "Invalid API Key provided") || s.stored("/gusto/stripe-key") != "" {
		t.Errorf("%v\n%s", err, out)
	}
}
