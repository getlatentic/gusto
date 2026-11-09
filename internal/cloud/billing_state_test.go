package cloud

import (
	"context"
	"github.com/getlatentic/gusto/internal/cloud/payments"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTwoEventsInOneSecondKeepTheSubscriptionAsStripeHasIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		stripe := newFakeStripe(t)
		stripe.subscription = `{"id":"sub_1","object":"subscription","customer":"cus_1","status":"active",
			"metadata":{"org":"acme-com"},"items":{"data":[{"id":"si_1","quantity":1}]}}`
		f := newFront(t, Settings{}, func(c *Config) { c.Billing = stripe.billing(c.Store) })
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		// Checkout makes the subscription incomplete and then active in one second, and Stripe sends the two the other way round.
		at := time.Now().Add(-time.Minute).Truncate(time.Second)
		for _, body := range [][]byte{
			subscriptionEvent("customer.subscription.updated", "acme-com", "sub_1", "active", 1, at),
			subscriptionEvent("customer.subscription.created", "acme-com", "sub_1", "incomplete", 1, at),
		} {
			if status, text := f.postEvent(body, signStripe(body, testWebhookSecret, time.Now())); status != http.StatusOK {
				t.Fatalf("%d %s", status, text)
			}
		}
		if b, _, _ := f.store.Plan(t.Context(), "acme-com"); b.State != payments.Active {
			t.Errorf("the older of two events in one second was kept: %+v", b)
		}
		if _, page := admin.get("/org"); !strings.Contains(page, "Paid: 1 seat") {
			t.Errorf("\n%s", page)
		}
	})
}

func TestAnAdminIsNotOfferedASecondSubscriptionWhileTheFirstAwaitsPayment(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		stripe := newFakeStripe(t)
		f := newFront(t, Settings{}, func(c *Config) { c.Billing = stripe.billing(c.Store) })
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		body := subscriptionEvent("customer.subscription.created", "acme-com", "sub_1", "incomplete", 1, time.Now())
		if status, text := f.postEvent(body, signStripe(body, testWebhookSecret, time.Now())); status != http.StatusOK {
			t.Fatalf("%d %s", status, text)
		}
		if _, page := admin.get("/org"); !strings.Contains(page, "Payment pending") || strings.Contains(page, "/org/billing/subscribe") {
			t.Errorf("an incomplete subscription offers a second:\n%s", page)
		}
	})
}

func TestAQuotaThatCouldNotBeWrittenIsWrittenByTheSweep(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		_, org, err := s.SignUp(t.Context(), Signup{Email: "ada@acme.com", OrgName: "Acme"})
		if err != nil {
			t.Fatal(err)
		}
		// A directory where the file goes makes the write fail.
		file := s.QuotaFile(org.Slug)
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(file, "in-the-way"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := s.Verify(t.Context(), "ada@acme.com"); err != nil {
			t.Fatalf("the address was confirmed, whatever became of the quota: %v", err)
		}
		if err := os.RemoveAll(file); err != nil {
			t.Fatal(err)
		}
		if err := s.applyDueQuotas(context.Background()); err != nil {
			t.Fatal(err)
		}
		if q, err := s.Quota(org.Slug); err != nil || q.Tasks != s.set.Limits.Verified.Tasks {
			t.Fatalf("the sweep did not write the verified quota: %+v %v", q, err)
		}
		if due, _ := count(context.Background(), s.db, `SELECT count(*) FROM orgs WHERE quota_due`); due != 0 {
			t.Errorf("%d quotas are still marked", due)
		}
	})
}
