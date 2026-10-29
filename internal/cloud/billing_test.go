package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const testWebhookSecret = "whsec_test"

// fakeStripe answers the three calls the seat billing makes, and keeps what it was sent.
type fakeStripe struct {
	mu       sync.Mutex
	calls    []string
	forms    []url.Values
	failing  bool
	listener *httptest.Server
}

func newFakeStripe(t *testing.T) *fakeStripe {
	f := &fakeStripe{}
	f.listener = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ := r.BasicAuth()
		r.ParseForm()
		f.mu.Lock()
		f.calls, f.forms = append(f.calls, r.URL.Path), append(f.forms, r.PostForm)
		failing := f.failing
		f.mu.Unlock()
		switch {
		case user != "sk_test":
			http.Error(w, `{"error":{"message":"Invalid API Key provided"}}`, http.StatusUnauthorized)
		case failing:
			http.Error(w, `{"error":{"message":"try again"}}`, http.StatusInternalServerError)
		case r.URL.Path == "/v1/checkout/sessions":
			w.Write([]byte(`{"id":"cs_1","url":"https://checkout.stripe.com/c/pay/cs_1"}`))
		case r.URL.Path == "/v1/billing_portal/sessions":
			w.Write([]byte(`{"id":"bps_1","url":"https://billing.stripe.com/p/session/bps_1"}`))
		case strings.HasPrefix(r.URL.Path, "/v1/subscription_items/"):
			w.Write([]byte(`{"id":"si_1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.listener.Close)
	return f
}

func (f *fakeStripe) stripe() *Stripe {
	return &Stripe{Key: "sk_test", Price: "price_seat", WebhookSecret: testWebhookSecret, SeatUSD: 25, API: f.listener.URL}
}

func (f *fakeStripe) last() (string, url.Values) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return "", nil
	}
	return f.calls[len(f.calls)-1], f.forms[len(f.forms)-1]
}

func (f *fakeStripe) fail(failing bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = failing
}

func (f *fakeStripe) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func signStripe(body []byte, secret string, at time.Time) string {
	stamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "." + string(body)))
	return "t=" + stamp + ",v1=" + hex.EncodeToString(mac.Sum(nil)) + ",v0=0000"
}

// subscriptionEvent is a Stripe event about an organization's subscription.
func subscriptionEvent(kind, org, sub, status string, seats int, at time.Time) []byte {
	event := map[string]any{"id": "evt_" + strconv.FormatInt(at.UnixNano(), 36), "type": kind, "created": at.Unix(),
		"data": map[string]any{"object": map[string]any{"id": sub, "object": "subscription", "customer": "cus_1", "status": status,
			"metadata": map[string]string{"org": org}, "items": map[string]any{"data": []map[string]any{{"id": "si_1", "quantity": seats}}}}}}
	body, _ := json.Marshal(event)
	return body
}

func TestOnlyAFreshV1SignatureWithTheSecretIsBelieved(t *testing.T) {
	now := time.Now()
	body := []byte(`{"id":"evt_1"}`)
	if !verifyStripe(signStripe(body, "s", now), body, "s", now) {
		t.Error("a good signature was refused")
	}
	for name, header := range map[string]string{
		"another secret": signStripe(body, "other", now),
		"too old":        signStripe(body, "s", now.Add(-6*time.Minute)),
		"only v0":        "t=" + strconv.FormatInt(now.Unix(), 10) + ",v0=" + strings.Split(signStripe(body, "s", now), "v1=")[1],
		"no time":        "v1=" + strings.Split(signStripe(body, "s", now), "v1=")[1],
		"empty":          "",
	} {
		if verifyStripe(header, body, "s", now) {
			t.Errorf("%s was believed", name)
		}
	}
	if verifyStripe(signStripe(body, "s", now), []byte(`{"id":"evt_2"}`), "s", now) {
		t.Error("a changed body was believed")
	}
}

func recorded(t *testing.T, s *Store, body []byte) (bool, error) {
	t.Helper()
	var event stripeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatal(err)
	}
	billing, about, err := billingOfEvent(event)
	if err != nil || !about {
		t.Fatalf("%v %v", about, err)
	}
	return s.RecordSubscription(billing)
}

func TestASubscriptionSetsThePaidQuotaForEachSeatAndAnEndedOneStopsNewTasks(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		_, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
		if kept, err := recorded(t, s, subscriptionEvent("customer.subscription.created", org.Slug, "sub_1", "active", 3, at)); !kept || err != nil {
			t.Fatalf("%v %v", kept, err)
		}
		q, _ := s.Quota(org.Slug)
		if q.Jobs != 600 || q.Members != 50 || !q.Invites || q.Concurrent != 4 {
			t.Errorf("three paid seats: %+v", q)
		}
		// An event Stripe made earlier, arriving late, changes nothing.
		if kept, _ := recorded(t, s, subscriptionEvent("customer.subscription.updated", org.Slug, "sub_1", "incomplete", 1, at.Add(-time.Minute))); kept {
			t.Error("an older event was kept")
		}
		if b, _, _ := s.Billing(org.Slug); b.Seats != 3 || b.Status != "active" || b.Customer != "cus_1" || b.Item != "si_1" {
			t.Errorf("%+v", b)
		}
		recorded(t, s, subscriptionEvent("customer.subscription.updated", org.Slug, "sub_1", "past_due", 3, at.Add(time.Hour)))
		if q, _ := s.Quota(org.Slug); q.Jobs != 600 {
			t.Errorf("while Stripe tries the card again the organization keeps working: %+v", q)
		}
		recorded(t, s, subscriptionEvent("customer.subscription.deleted", org.Slug, "sub_1", "canceled", 3, at.Add(2*time.Hour)))
		q, _ = s.Quota(org.Slug)
		if q.Jobs != 0 || q.Concurrent != 0 || q.Invites || q.Members != 50 || q.Computers != 50 || q.Note != lapsedNote {
			t.Errorf("an ended subscription starts nothing and removes nobody: %+v", q)
		}
		if err := q.Refuse(0); err == nil || !strings.Contains(err.Error(), "renew") {
			t.Errorf("%v", err)
		}
		// A new subscription after the old one ended is kept.
		if kept, err := recorded(t, s, subscriptionEvent("customer.subscription.created", org.Slug, "sub_2", "active", 1, at.Add(3*time.Hour))); !kept || err != nil {
			t.Errorf("%v %v", kept, err)
		}
	})
}

func TestASecondSubscriptionForAnOrganizationThatPaysIsRefusedAndNoticed(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		_, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		at := time.Now().UTC().Truncate(time.Second)
		recorded(t, s, subscriptionEvent("customer.subscription.created", org.Slug, "sub_1", "active", 1, at))
		if _, err := recorded(t, s, subscriptionEvent("customer.subscription.created", org.Slug, "sub_2", "active", 1, at.Add(time.Second))); !errors.Is(err, errSecondSubscription) {
			t.Errorf("%v", err)
		}
		if b, _, _ := s.Billing(org.Slug); b.Subscription != "sub_1" {
			t.Errorf("the first subscription stands: %+v", b)
		}
		if kept, _ := recorded(t, s, subscriptionEvent("customer.subscription.created", "nobody-com", "sub_3", "active", 1, at)); kept {
			t.Error("an event for an organization that is not there was kept")
		}
	})
}

func TestConfirmingAnAddressDoesNotTakeAPaidOrganizationOffItsPlan(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		_, org, _ := s.SignUp(Signup{Email: "ada@acme.com", OrgName: "Acme"})
		recorded(t, s, subscriptionEvent("customer.subscription.created", org.Slug, "sub_1", "active", 2, time.Now()))
		if err := s.Verify("ada@acme.com"); err != nil {
			t.Fatal(err)
		}
		if q, _ := s.Quota(org.Slug); q.Jobs != 400 {
			t.Errorf("%+v", q)
		}
	})
}

func (f *front) postEvent(body []byte, signature string) (int, string) {
	f.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, f.http.URL+"/stripe/webhook", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signature)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(text)
}

func TestStripesEventsArriveSignedAndChangeTheOrganizationsPlan(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		stripe := newFakeStripe(t)
		f := newFront(t, Settings{}, func(c *Config) { c.Stripe = stripe.stripe() })
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		body := subscriptionEvent("customer.subscription.created", "acme-com", "sub_1", "active", 1, time.Now())
		if status, _ := f.postEvent(body, signStripe(body, "wrong", time.Now())); status != http.StatusBadRequest {
			t.Errorf("an unsigned event was taken: %d", status)
		}
		if _, page := admin.get("/org"); !strings.Contains(page, "Free trial") || !strings.Contains(page, "Subscribe: 1 seat at $25 a month each") {
			t.Fatalf("before paying:\n%s", page)
		}
		if status, text := f.postEvent(body, signStripe(body, testWebhookSecret, time.Now())); status != http.StatusOK {
			t.Fatalf("%d %s", status, text)
		}
		_, page := admin.get("/org")
		if !strings.Contains(page, "Paid: 1 seat at $25 a month each") || strings.Contains(page, "/org/billing/subscribe") || !strings.Contains(page, "/org/billing/manage") {
			t.Errorf("after paying:\n%s", page)
		}
		// An event bigger than a form, as Stripe's are, is taken.
		big := subscriptionEvent("customer.subscription.updated", "acme-com", "sub_1", "active", 1, time.Now())
		big = []byte(strings.Replace(string(big), `"object":"subscription"`, `"object":"subscription","description":"`+strings.Repeat("x", 40<<10)+`"`, 1))
		if status, text := f.postEvent(big, signStripe(big, testWebhookSecret, time.Now())); status != http.StatusOK {
			t.Errorf("a large event: %d %s", status, text)
		}
		other := []byte(`{"id":"evt_x","type":"invoice.paid","created":1,"data":{"object":{}}}`)
		if status, _ := f.postEvent(other, signStripe(other, testWebhookSecret, time.Now())); status != http.StatusOK {
			t.Errorf("an event about something else is answered and ignored: %d", status)
		}
	})
}

func TestAnAdminSubscribesForASeatForEachMemberAndARunnerCannot(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		stripe := newFakeStripe(t)
		f := newFront(t, Settings{}, func(c *Config) { c.Stripe = stripe.stripe() })
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		bob := f.invitedAndJoined(admin, "bob@acme.com")
		status, _, header := admin.post("/org/billing/subscribe", nil)
		if status != http.StatusSeeOther || header.Get("Location") != "https://checkout.stripe.com/c/pay/cs_1" {
			t.Fatalf("%d %v", status, header)
		}
		if !strings.Contains(header.Get("Content-Security-Policy"), "form-action 'self' https://checkout.stripe.com") {
			t.Errorf("the browser may follow the form to Stripe: %s", header.Get("Content-Security-Policy"))
		}
		path, form := stripe.last()
		if path != "/v1/checkout/sessions" || form.Get("mode") != "subscription" || form.Get("line_items[0][price]") != "price_seat" ||
			form.Get("line_items[0][quantity]") != "2" || form.Get("client_reference_id") != "acme-com" ||
			form.Get("subscription_data[metadata][org]") != "acme-com" || form.Get("customer_email") != "ada@acme.com" ||
			form.Get("success_url") != "http://cloud.test/org?org=acme-com&billing=started" {
			t.Errorf("%s %v", path, form)
		}
		calls := stripe.count()
		if status, _, _ := bob.post("/org/billing/subscribe", nil); status != http.StatusForbidden || stripe.count() != calls {
			t.Errorf("a runner subscribed: %d", status)
		}
		if _, page := admin.get("/org?billing=started"); !strings.Contains(page, "once Stripe confirms the payment") {
			t.Errorf("%s", page)
		}
		stripe.fail(true)
		if status, page, _ := admin.post("/org/billing/subscribe", nil); status != http.StatusBadGateway || !strings.Contains(page, "Payments are not answering") {
			t.Errorf("%d", status)
		}
	})
}

func TestAPayingAdminManagesTheirCardAtStripe(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		stripe := newFakeStripe(t)
		f := newFront(t, Settings{}, func(c *Config) { c.Stripe = stripe.stripe() })
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		if status, _, _ := admin.post("/org/billing/manage", nil); status != http.StatusNotFound {
			t.Errorf("an organization with no customer has no portal: %d", status)
		}
		recorded(t, f.store, subscriptionEvent("customer.subscription.created", "acme-com", "sub_1", "active", 1, time.Now()))
		status, _, header := admin.post("/org/billing/manage", nil)
		if status != http.StatusSeeOther || header.Get("Location") != "https://billing.stripe.com/p/session/bps_1" {
			t.Fatalf("%d %v", status, header)
		}
		if path, form := stripe.last(); path != "/v1/billing_portal/sessions" || form.Get("customer") != "cus_1" || form.Get("return_url") != "http://cloud.test/org?org=acme-com" {
			t.Errorf("%s %v", path, form)
		}
	})
}

func TestTheSeatsBilledFollowTheMembersAndAFailedChangeIsTriedAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		stripe := newFakeStripe(t)
		s := open(t, Settings{})
		ada, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		recorded(t, s, subscriptionEvent("customer.subscription.created", org.Slug, "sub_1", "active", 1, time.Now()))
		seats := NewSeatSync(s, stripe.stripe())
		ctx := context.Background()
		if n := seats.Once(ctx); n != 0 || stripe.count() != 0 {
			t.Errorf("seats that match are left alone: %d", n)
		}
		joined(t, s, org, ada, "bob@acme.com")
		joined(t, s, org, ada, "cy@acme.com")
		stripe.fail(true)
		if n := seats.Once(ctx); n != 0 {
			t.Errorf("a failed change was counted: %d", n)
		}
		if b, _, _ := s.Billing(org.Slug); b.Seats != 1 {
			t.Errorf("a change Stripe refused was recorded: %+v", b)
		}
		stripe.fail(false)
		if n := seats.Once(ctx); n != 1 {
			t.Fatalf("%d", n)
		}
		path, form := stripe.last()
		if path != "/v1/subscription_items/si_1" || form.Get("quantity") != "3" || form.Get("proration_behavior") != "create_prorations" {
			t.Errorf("%s %v", path, form)
		}
		if q, _ := s.Quota(org.Slug); q.Jobs != 600 {
			t.Errorf("the quota follows the seats: %+v", q)
		}
		s.Remove(org.Slug, ada, "cy@acme.com")
		seats.Once(ctx)
		if _, form := stripe.last(); form.Get("quantity") != "2" {
			t.Errorf("%v", form)
		}
		// An organization whose subscription ended cannot invite, and is not billed for anybody.
		recorded(t, s, subscriptionEvent("customer.subscription.deleted", org.Slug, "sub_1", "canceled", 2, time.Now().Add(time.Hour)))
		if _, err := s.Invite(org.Slug, ada, "dee@acme.com"); !errors.Is(err, ErrLapsed) {
			t.Errorf("%v", err)
		}
		s.Remove(org.Slug, ada, "bob@acme.com")
		calls := stripe.count()
		if n := seats.Once(ctx); n != 0 || stripe.count() != calls {
			t.Errorf("an ended subscription was changed: %d", n)
		}
	})
}

func TestADemoIsNeverOfferedAPlan(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		stripe := newFakeStripe(t)
		f := newFront(t, demoSet, func(c *Config) { c.Stripe = stripe.stripe() })
		person, org, err := f.store.StartDemo("Ada", "ada@acme.com", "Acme")
		if err != nil {
			t.Fatal(err)
		}
		if p := f.server.planOf(org, person); p != nil {
			t.Errorf("%+v", p)
		}
		if err := f.store.EndDemo(org.Slug, "done"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestBillingIsOffUntilItIsSetUp(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		if _, page := admin.get("/org"); strings.Contains(page, "Plan") || strings.Contains(page, "Subscribe") {
			t.Errorf("%s", page)
		}
		body := subscriptionEvent("customer.subscription.created", "acme-com", "sub_1", "active", 1, time.Now())
		if status, _ := f.postEvent(body, signStripe(body, testWebhookSecret, time.Now())); status != http.StatusNotFound {
			t.Errorf("%d", status)
		}
		if status, _, _ := admin.post("/org/billing/subscribe", nil); status != http.StatusNotFound {
			t.Errorf("%d", status)
		}
	})
}

func TestSeatWords(t *testing.T) {
	for seats, want := range map[int]string{1: "1 seat at $25 a month each", 4: "4 seats at $25 a month each"} {
		if got := seatWords(seats, 25); got != want {
			t.Errorf("%d: %s", seats, got)
		}
	}
}

// Every organization has the starting credit on the models gusto provides, a demo a smaller one, and a front door that starts
// writes every quota again so organizations made before a change in the limits have it too.
func TestEveryOrganizationHasACreditAndAStartWritesEveryQuotaAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s, _ := clocked(t, demoSet)
		_, org, _ := s.SignUp(Signup{Email: "ada@acme.com", OrgName: "Acme"})
		_, demo, _ := s.StartDemo("Bo", "bo@globex.com", "Globex")
		if q, _ := s.Quota(org.Slug); q.CreditUSD != StartingCredit {
			t.Errorf("%+v", q)
		}
		if q, _ := s.Quota(demo.Slug); q.CreditUSD != 0.5 {
			t.Errorf("%+v", q)
		}
		old := DefaultLimits().Trial
		old.CreditUSD = 0
		if err := s.setQuota(org.Slug, old); err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyQuotas(); err != nil {
			t.Fatal(err)
		}
		if q, _ := s.Quota(org.Slug); q.CreditUSD != StartingCredit {
			t.Errorf("a quota written before the credit was not written again: %+v", q)
		}
	})
}

func TestTheOrganizationPageSaysWhatIsLeftOfTheCredit(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		if _, page := admin.get("/org"); !strings.Contains(page, "$5.00 of $5.00 left on gusto's models, then add your own key in Admin, Models") {
			t.Errorf("a new organization's credit:\n%s", page)
		}
		os.WriteFile(filepath.Join(f.store.DataDir("acme-com"), "credit-used.json"), []byte(`{"usd":1.8}`), 0o600)
		if _, page := admin.get("/org"); !strings.Contains(page, "$3.20 of $5.00 left") {
			t.Errorf("after the server used some:\n%s", page)
		}
	})
}
