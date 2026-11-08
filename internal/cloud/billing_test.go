package cloud

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/quota"
)

// billed is a front door that sells by PayPal against a fake, with one organization whose admin is signed in.
type billed struct {
	t     *testing.T
	front *front
	fake  *fakePayPal
	admin *browser
	org   string
	hook  string
}

func newBilled(t *testing.T) *billed {
	t.Helper()
	fake := newFakePayPal(t)
	f := newFront(t, Settings{AllowFreeMail: true}, func(c *Config) { c.Billing = &Billing{Store: c.Store, PayPal: fake.client()} })
	admin := f.browser()
	if code, _, _ := admin.post("/signup", signup("ada@acme.com", "Acme")); code != 303 {
		t.Fatalf("sign-up answered %d", code)
	}
	person, _, _ := f.store.Person("ada@acme.com")
	if _, err := f.store.Verify(person.Email); err != nil {
		t.Fatal(err)
	}
	return &billed{t: t, front: f, fake: fake, admin: admin, org: person.Org, hook: f.http.URL + billingWebhookPath}
}

// subscribe makes the admin press Subscribe, and returns where PayPal sends them and the subscription's id.
func (b *billed) subscribe() (location, id string) {
	b.t.Helper()
	code, _, header := b.admin.post("/org/billing/subscribe", nil)
	if code != 303 || !strings.HasPrefix(header.Get("Location"), "https://www.sandbox.paypal.com/") {
		b.t.Fatalf("subscribing answered %d to %q", code, header.Get("Location"))
	}
	record, found, err := b.front.store.BillingOf(b.org)
	if err != nil || !found {
		b.t.Fatalf("no billing record: %v", err)
	}
	return header.Get("Location"), record.SubscriptionID
}

// activate is the payer approving at PayPal and PayPal's webhook saying so.
func (b *billed) activate(id, event string) {
	b.t.Helper()
	b.fake.activate(id)
	b.deliver(event, "BILLING.SUBSCRIPTION.ACTIVATED", id, 204)
}

func (b *billed) deliver(event, kind, subscription string, want int) {
	b.t.Helper()
	resource := map[string]any{"id": subscription}
	if strings.HasPrefix(kind, "PAYMENT.SALE.") {
		resource = map[string]any{"id": "SALE-1", "billing_agreement_id": subscription}
	}
	if got := b.fake.deliver(b.hook, event, kind, resource); got != want {
		b.t.Fatalf("delivering %s answered %d, want %d", kind, got, want)
	}
}

func (b *billed) record() BillingRecord {
	b.t.Helper()
	record, _, err := b.front.store.BillingOf(b.org)
	if err != nil {
		b.t.Fatal(err)
	}
	return record
}

func (b *billed) quota() quota.Quota {
	b.t.Helper()
	q, err := b.front.store.Quota(b.org)
	if err != nil {
		b.t.Fatal(err)
	}
	return q
}

func TestSubscribingWaitsForThePayerAndChangesNothingUntilPayPalSaysItIsActive(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		before := b.quota()
		location, id := b.subscribe()
		if !strings.Contains(location, id) || b.record().State != BillingPending || b.record().Seats != 1 {
			t.Fatalf("%q %+v", location, b.record())
		}
		if b.quota() != before {
			t.Errorf("a subscription nobody has approved changed the quota: %+v", b.quota())
		}
		if _, again := b.subscribe(); again != id || b.fake.count("POST /v1/billing/subscriptions") != 1 {
			t.Errorf("pressing Subscribe twice made a second subscription")
		}
		b.activate(id, "WH-1")
		got := b.quota()
		if b.record().State != BillingActive || got.Members != 1 || got.Computers != 1 || !got.Invites || got.Jobs != DefaultPlanQuota().Jobs {
			t.Errorf("%+v %+v", b.record(), got)
		}
	})
}

func TestThePlanIsMadeOnceAndIsTheSeatPriceAMonth(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		b.subscribe()
		if id, _, _ := b.front.store.setting(settingPayPalPlan); id != "P-PLAN" {
			t.Errorf("the plan was not kept: %q", id)
		}
		if b.fake.count("POST /v1/billing/plans") != 1 || b.fake.count("POST /v1/catalogs/products") != 1 {
			t.Errorf("the plan was made more than once")
		}
	})
}

func TestADeliveryThatPayPalDidNotSignIsRefusedAndAppliesNothing(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.fake.activate(id)
		body := []byte(`{"id":"WH-FORGED","event_type":"BILLING.SUBSCRIPTION.ACTIVATED","resource":{"id":"` + id + `"}}`)
		if code := post(t, b.hook, "forged", "WH-FORGED", body); code != 401 {
			t.Errorf("a forged delivery answered %d", code)
		}
		if code := post(t, b.hook, "", "", body); code != 400 {
			t.Errorf("a delivery without PayPal's headers answered %d", code)
		}
		if b.record().State != BillingPending {
			t.Errorf("a forged delivery changed the state: %+v", b.record())
		}
	})
}

func TestAnEventPayPalSendsTwiceIsAppliedOnce(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		reads := b.fake.count("GET /v1/billing/subscriptions/" + id)
		b.deliver("WH-1", "BILLING.SUBSCRIPTION.ACTIVATED", id, 204)
		if b.fake.count("GET /v1/billing/subscriptions/"+id) != reads || b.record().State != BillingActive {
			t.Errorf("the second delivery was applied again")
		}
	})
}

func TestAnEventForASubscriptionGustoNeverMadeChangesNothing(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		b.fake.subs["I-OTHER"] = &fakeSub{id: "I-OTHER", status: "ACTIVE", quantity: 9, custom: b.org}
		b.deliver("WH-9", "BILLING.SUBSCRIPTION.ACTIVATED", "I-OTHER", 204)
		if _, found, _ := b.front.store.BillingOf(b.org); found {
			t.Errorf("a stranger's subscription became the organization's")
		}
	})
}

func TestAFailedPaymentIsPastDueAndKeepsWorkingUntilPayPalSuspendsIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		paid := b.quota()
		b.fake.failPayment(id)
		b.deliver("WH-2", "BILLING.SUBSCRIPTION.PAYMENT.FAILED", id, 204)
		if b.record().State != BillingPastDue || b.quota() != paid {
			t.Errorf("past due must keep working: %+v %+v", b.record(), b.quota())
		}
		b.fake.recover(id)
		b.deliver("WH-3", "PAYMENT.SALE.COMPLETED", id, 204)
		if b.record().State != BillingActive {
			t.Errorf("a payment that went through left it %s", b.record().State)
		}
		b.fake.failPayment(id)
		b.fake.suspend(id)
		b.deliver("WH-4", "BILLING.SUBSCRIPTION.SUSPENDED", id, 204)
		got := b.quota()
		if b.record().State != BillingSuspended || got.Jobs != 0 || got.Invites || got.Refuse(0) == nil || !errors.Is(got.Refuse(0), quota.ErrExceeded) {
			t.Errorf("suspended must stop new tasks: %+v %+v", b.record(), got)
		}
	})
}

func TestSuspendingNeverDeletesAnythingAndSubscribingAgainRestoresIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		b.fake.suspend(id)
		b.deliver("WH-2", "BILLING.SUBSCRIPTION.SUSPENDED", id, 204)
		if people, _ := b.front.store.Members(b.org); len(people) != 1 {
			t.Errorf("suspending removed people: %v", people)
		}
		if _, found, _ := b.front.store.Org(b.org); !found {
			t.Errorf("suspending removed the organization")
		}
		_, next := b.subscribe()
		if next == id {
			t.Fatalf("a suspended organization must be able to start a new subscription")
		}
		b.activate(next, "WH-3")
		if b.record().State != BillingActive || b.quota().Jobs == 0 {
			t.Errorf("%+v %+v", b.record(), b.quota())
		}
	})
}

func TestCancellingEndsTheSubscriptionAndSuspends(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		if code, _, _ := b.admin.post("/org/billing/cancel", nil); code != 303 {
			t.Fatalf("cancelling answered %d", code)
		}
		if b.record().State != BillingSuspended || b.fake.subs[id].status != "CANCELLED" {
			t.Errorf("%+v %s", b.record(), b.fake.subs[id].status)
		}
	})
}

func TestAnOlderAnswerFromPayPalNeverOverwritesANewerOne(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		newer := b.record().SourceTime
		stale := Subscription{ID: id, Status: "ACTIVE", Quantity: "7", UpdateTime: newer.Add(-time.Hour)}
		_, record, err := b.front.store.applySubscription(t.Context(), b.org, stale, BillingActive)
		if err != nil || record.Seats != 1 {
			t.Errorf("a stale answer was applied: %v %+v", err, record)
		}
	})
}

func TestOnlyAnAdminMayStartCancelOrApproveAndARunnerSeesTheState(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		s := b.front.store
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		ada, _, _ := s.Person("ada@acme.com")
		s.setQuota(b.org, quota.Quota{Jobs: 10, Invites: true, Members: 5, Computers: 5})
		secret, _ := s.Invite(b.org, ada, "bob@acme.com")
		runner := b.front.browser()
		if code, _, _ := runner.post("/invite", url.Values{"t": {secret}}); code != 303 {
			t.Fatalf("accepting answered %d", code)
		}
		for _, path := range []string{"/org/billing/subscribe", "/org/billing/cancel", "/org/billing/proposals/approve", "/org/billing/proposals/decline", "/org/billing/agent-token"} {
			if code, _, _ := runner.post(path, url.Values{"id": {"x"}}); code != 403 {
				t.Errorf("%s as a runner answered %d", path, code)
			}
		}
		if _, page := runner.get("/org"); !strings.Contains(page, "Paid, 1 seats") || strings.Contains(page, "Cancel the subscription") {
			t.Errorf("a runner sees the state and no buttons:\n%s", page)
		}
	})
}

func TestTheOrganizationPageShowsBillingAndProposalsToAnAdmin(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		if _, page := b.admin.get("/org"); !strings.Contains(page, "Subscribe with PayPal") || !strings.Contains(page, "$25.00 a seat a month") {
			t.Errorf("%s", page)
		}
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		b.front.store.Propose(b.org, 3, "two colleagues are joining", "agent")
		if _, page := b.admin.get("/org"); !strings.Contains(page, "An agent proposes 3 seats") || !strings.Contains(page, "Approve and go to PayPal") {
			t.Errorf("%s", page)
		}
	})
}

func TestComingBackFromPayPalReadsTheSubscriptionAtOnce(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.fake.activate(id)
		if code, _ := b.admin.get("/org/billing/return?subscription_id=I-SOMEONE-ELSES"); code != 303 {
			t.Fatalf("returning answered %d", code)
		}
		if b.record().State != BillingActive {
			t.Errorf("the return did not read the organization's own subscription: %+v", b.record())
		}
	})
}

func TestWithoutPayPalThereAreNoBillingPagesOrEndpoints(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{AllowFreeMail: true}, nil)
		admin := f.browser()
		admin.post("/signup", signup("ada@acme.com", "Acme"))
		if code, _, _ := admin.post("/org/billing/subscribe", nil); code != 404 && code != 405 {
			t.Errorf("subscribe answered %d", code)
		}
		if code := post(t, f.http.URL+billingWebhookPath, "x", "x", []byte(`{}`)); code != 404 && code != 405 {
			t.Errorf("the webhook answered %d", code)
		}
		if _, page := admin.get("/org"); strings.Contains(page, "Billing") {
			t.Errorf("the page offers billing")
		}
	})
}

func TestAPendingSubscriptionThatPayPalHasEndedIsReplacedAndOneThatIsPayingIsNot(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		earned := b.quota()
		_, first := b.subscribe()
		b.fake.set(first, func(s *fakeSub) { s.status = "EXPIRED" })
		_, second := b.subscribe()
		if b.quota() != earned {
			t.Errorf("a subscription nobody paid for changed the quota: %+v", b.quota())
		}
		if second == first || b.record().State != BillingPending || b.record().SubscriptionID != second {
			t.Fatalf("an expired subscription was not replaced: %+v", b.record())
		}
		b.fake.activate(second)
		if code, page, _ := b.admin.post("/org/billing/subscribe", nil); code != 409 || !strings.Contains(page, "already has a subscription") {
			t.Errorf("subscribing while paying answered %d", code)
		}
		if b.record().State != BillingActive {
			t.Errorf("the paying subscription was not read in: %+v", b.record())
		}
	})
}
