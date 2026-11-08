package cloud

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTheClientTalksOnlyToThePayPalSandbox(t *testing.T) {
	if got := NewPayPalClient(PayPalConfig{}).base; got != "https://api-m.sandbox.paypal.com" {
		t.Errorf("the client's address is %q", got)
	}
}

func TestOneAccessTokenServesManyCallsAndIsRenewedBeforeItEnds(t *testing.T) {
	fake := newFakePayPal(t)
	client := fake.client()
	now := fake.clock
	client.now = func() time.Time { return now }
	for range 3 {
		if _, err := client.EnsurePlan(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if fake.tokens != 1 {
		t.Errorf("%d tokens were fetched for one burst", fake.tokens)
	}
	now = now.Add(9 * time.Hour)
	if _, err := client.EnsurePlan(t.Context()); err != nil || fake.tokens != 2 {
		t.Errorf("an ended token was not renewed: %v %d", err, fake.tokens)
	}
}

func TestARefusalFromPayPalCarriesItsDebugIDAndNeverTheSecret(t *testing.T) {
	fake := newFakePayPal(t)
	client := fake.client()
	client.cfg.ClientSecret = "the-wrong-secret"
	_, err := client.Subscription(t.Context(), "I-1")
	var refusal *PayPalError
	if !errors.As(err, &refusal) || refusal.Status != 401 || refusal.DebugID != "debug-1" {
		t.Fatalf("%v", err)
	}
	if strings.Contains(err.Error(), "the-wrong-secret") || strings.Contains(err.Error(), fakeSecret) {
		t.Errorf("the error holds a secret: %v", err)
	}
	client.cfg.ClientSecret = fakeSecret
	if _, err := client.Subscription(t.Context(), "I-MISSING"); !errors.As(err, &refusal) || refusal.Status != 404 {
		t.Errorf("%v", err)
	}
}

func TestAWebhookBodyGoesBackToPayPalByteForByte(t *testing.T) {
	fake := newFakePayPal(t)
	body := []byte("{\n  \"id\": \"WH-X\",   \"event_type\": \"BILLING.SUBSCRIPTION.UPDATED\", \"resource\": {\"id\":\"I-1\", \"note\":\"café \\u00e9 </tag>\"}\n}\n")
	fake.signed["WH-X"] = []byte(strings.TrimSpace(string(body)))
	headers := WebhookHeaders{Algo: "a", CertURL: "c", ID: "WH-X", Signature: "sig-WH-X", Time: "t"}
	genuine, err := fake.client().VerifyWebhook(t.Context(), headers, body)
	if err != nil || !genuine {
		t.Errorf("a body with its own spacing and escapes came back changed: %v %v", genuine, err)
	}
	headers.Signature = "tampered"
	if genuine, _ := fake.client().VerifyWebhook(t.Context(), headers, body); genuine {
		t.Errorf("a wrong signature was accepted")
	}
}

func TestStateOfASubscriptionIsWhatItsStatusAndFailedPaymentsSay(t *testing.T) {
	cases := []struct {
		status string
		failed int
		want   PayPalState
		known  bool
	}{
		{"APPROVAL_PENDING", 0, PayPalPending, true}, {"APPROVED", 0, PayPalPending, true},
		{"ACTIVE", 0, PayPalActive, true}, {"ACTIVE", 2, PayPalPastDue, true},
		{"SUSPENDED", 0, PayPalSuspended, true}, {"CANCELLED", 0, PayPalSuspended, true}, {"EXPIRED", 0, PayPalSuspended, true},
		{"SOMETHING_NEW", 0, "", false},
	}
	for _, c := range cases {
		if got, known := stateOf(Subscription{Status: c.status, FailedPayments: c.failed}); got != c.want || known != c.known {
			t.Errorf("%s with %d failed: %q %v", c.status, c.failed, got, known)
		}
	}
}

// TestPayPalSandbox reaches the real sandbox with the credentials GUSTO_PAYPAL_CLIENT_ID and
// GUSTO_PAYPAL_CLIENT_SECRET of a sandbox app. It makes the plan and a subscription that waits for a payer, and
// moves no money: approval needs a sandbox buyer in a browser.
func TestPayPalSandbox(t *testing.T) {
	id, secret := os.Getenv("GUSTO_PAYPAL_CLIENT_ID"), os.Getenv("GUSTO_PAYPAL_CLIENT_SECRET")
	if id == "" || secret == "" {
		t.Skip("no sandbox credentials in the environment")
	}
	client := NewPayPalClient(PayPalConfig{ClientID: id, ClientSecret: secret, WebhookID: "unused"})
	plan, err := client.EnsurePlan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	made, err := client.CreateSubscription(t.Context(), plan, 2, "sandbox-test", "https://cloud.example/org/paypal/return", "https://cloud.example/org", "gusto-sandbox-test-"+time.Now().Format("150405"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(made.ApproveURL, "sandbox.paypal.com") {
		t.Errorf("the approval address is not the sandbox's: %s", made.ApproveURL)
	}
	read, err := client.Subscription(t.Context(), made.ID)
	if err != nil || read.Status != "APPROVAL_PENDING" || read.Seats(0) != 2 || read.CustomID != "sandbox-test" {
		t.Errorf("%+v %v", read, err)
	}
}
