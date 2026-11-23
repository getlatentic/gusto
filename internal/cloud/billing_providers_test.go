package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/payments"
)

// ledger is a payment processor that is not Stripe and not PayPal, and does only what every processor must: start a subscription,
// say what it is, and send notices signed with a shared secret. That gusto cloud sells through it with no change of its own is
// what makes processors pluggable.
type ledger struct {
	mu      sync.Mutex
	name    string
	subs    map[string]payments.Subscription
	started int
}

func newLedger(name string) *ledger {
	return &ledger{name: name, subs: map[string]payments.Subscription{}}
}

func (l *ledger) Name() string      { return l.name }
func (l *ledger) Label() string     { return strings.ToUpper(l.name) }
func (l *ledger) Price() string     { return "$30 a month each" }
func (l *ledger) Origins() []string { return []string{"https://pay." + l.name + ".example"} }

func (l *ledger) Start(_ context.Context, req payments.StartRequest) (payments.Started, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.started++
	ref := l.name + "-sub-" + req.Org
	l.subs[ref] = payments.Subscription{Ref: ref, State: payments.Pending, Seats: req.Seats, At: time.Now()}
	return payments.Started{URL: "https://pay." + l.name + ".example/" + ref, Ref: ref}, nil
}

func (l *ledger) Fetch(_ context.Context, ref string) (payments.Subscription, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.subs[ref], nil
}

func (l *ledger) Webhook(_ context.Context, d payments.Delivery) (payments.Notice, error) {
	if d.Header.Get("X-Ledger-Secret") != "shh" {
		return payments.Notice{}, payments.ErrBadSignature
	}
	var in struct{ ID, Ref string }
	if err := json.Unmarshal(d.Body, &in); err != nil {
		return payments.Notice{}, payments.ErrMalformed
	}
	return payments.Notice{ID: in.ID, Kind: "changed", Ref: in.Ref}, nil
}

// pay is the payer paying at the processor.
func (l *ledger) pay(ref string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	sub := l.subs[ref]
	sub.State, sub.At = payments.Active, time.Now().Add(time.Second)
	l.subs[ref] = sub
}

func (f *front) notify(provider, secret, id, ref string) int {
	f.t.Helper()
	body, _ := json.Marshal(map[string]string{"ID": id, "Ref": ref})
	req, _ := http.NewRequest(http.MethodPost, f.http.URL+webhookPrefix+provider, strings.NewReader(string(body)))
	req.Header.Set("X-Ledger-Secret", secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestAProcessorThatDoesOnlyWhatEveryProcessorMustIsSoldThroughWithNoOtherChange(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		pay := newLedger("ledger")
		f := newFront(t, Settings{}, func(c *Config) { c.Billing = NewBilling(c.Store, pay) })
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		status, _, header := admin.post("/org/billing/subscribe", nil)
		if status != http.StatusSeeOther || header.Get("Location") != "https://pay.ledger.example/ledger-sub-acme-com" {
			t.Fatalf("%d %v", status, header)
		}
		if !strings.Contains(header.Get("Content-Security-Policy"), "https://pay.ledger.example") {
			t.Errorf("the browser may not follow the form to the processor: %s", header.Get("Content-Security-Policy"))
		}
		if _, page := admin.get("/org"); !strings.Contains(page, "Payment pending") || !strings.Contains(page, "Subscribe: 1 seat at $30 a month each") {
			t.Errorf("a payment waits, and the payer may go back to it:\n%s", page)
		}
		if status, _, _ := admin.post("/org/billing/subscribe", nil); status != http.StatusSeeOther || pay.started != 1 {
			t.Errorf("pressing Subscribe again made a second subscription: %d, %d started", status, pay.started)
		}
		if got := f.notify("ledger", "wrong", "n1", "ledger-sub-acme-com"); got != http.StatusUnauthorized {
			t.Errorf("an unsigned notice: %d", got)
		}
		pay.pay("ledger-sub-acme-com")
		if got := f.notify("ledger", "shh", "n1", "ledger-sub-acme-com"); got != http.StatusOK {
			t.Fatalf("a signed notice: %d", got)
		}
		plan, _, _ := f.store.Plan(t.Context(), "acme-com")
		q, _ := f.store.Quota("acme-com")
		if plan.State != payments.Active || plan.Provider != "ledger" || !plan.Fixed || q.Members != 1 || q.Tasks != f.store.paidQuota(1).Tasks {
			t.Errorf("%+v %+v", plan, q)
		}
		_, page := admin.get("/org")
		if !strings.Contains(page, "Paid: 1 seat at $30 a month each") || strings.Contains(page, "Card, invoices and plan") || strings.Contains(page, "Cancel the subscription") {
			t.Errorf("a processor with no portal and no cancel offers neither:\n%s", page)
		}
		for _, path := range []string{"/org/billing/cancel", "/org/billing/manage"} {
			if status, _, _ := admin.post(path, nil); status != http.StatusConflict && status != http.StatusNotFound {
				t.Errorf("%s: %d", path, status)
			}
		}
		if _, err := f.store.Propose("acme-com", 3, "growing", "agent"); err != nil {
			t.Errorf("seats this processor cannot revise are still proposed and refused at approval: %v", err)
		}
	})
}

func TestSeveralProcessorsAreOfferedAndAnOrganizationPaysThroughOne(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		first, second := newLedger("ledger"), newLedger("books")
		f := newFront(t, Settings{}, func(c *Config) { c.Billing = NewBilling(c.Store, first, second) })
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		if _, page := admin.get("/org"); !strings.Contains(page, "Subscribe with LEDGER") || !strings.Contains(page, "Subscribe with BOOKS") {
			t.Errorf("both are offered:\n%s", page)
		}
		if status, _, _ := admin.post("/org/billing/subscribe", nil); status != http.StatusConflict {
			t.Errorf("with two processors one must be chosen: %d", status)
		}
		form := map[string][]string{"provider": {"books"}}
		if status, _, header := admin.post("/org/billing/subscribe", form); status != http.StatusSeeOther || !strings.Contains(header.Get("Location"), "pay.books.example") {
			t.Fatalf("%d %v", status, header)
		}
		second.pay("books-sub-acme-com")
		if got := f.notify("books", "shh", "n1", "books-sub-acme-com"); got != http.StatusOK {
			t.Fatalf("%d", got)
		}
		if status, _, _ := admin.post("/org/billing/subscribe", map[string][]string{"provider": {"ledger"}}); status != http.StatusConflict || first.started != 0 {
			t.Errorf("an organization that pays was sold a second subscription: %d, %d started", status, first.started)
		}
		if got := f.notify("ledger", "shh", "n2", "books-sub-acme-com"); got != http.StatusOK {
			t.Errorf("a notice at one processor's address about another's subscription is ignored: %d", got)
		}
		if got := f.notify("nobody", "shh", "n3", "x"); got != http.StatusNotFound {
			t.Errorf("a processor nobody set up: %d", got)
		}
		if plan, _, _ := f.store.Plan(t.Context(), "acme-com"); plan.Provider != "books" || !plan.State.Paying() {
			t.Errorf("%+v", plan)
		}
	})
}

func TestAProcessorWithTheSameNameTwiceIsAMistakeOfTheProgramThatBuildsTheServer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("two processors named ledger were taken")
		}
	}()
	NewBilling(nil, newLedger("ledger"), newLedger("ledger"))
}

func TestAnOrganizationThatPaysIsNotSoldASecondSubscriptionAndAnEndedOneMayBeReplaced(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		_, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		at := time.Now().UTC().Truncate(time.Second)
		first := payments.Subscription{Ref: "a", State: payments.Active, Seats: 1, At: at}
		if kept, err := s.apply(t.Context(), "ledger", org.Slug, first, true); !kept || err != nil {
			t.Fatalf("%v %v", kept, err)
		}
		other := payments.Subscription{Ref: "b", State: payments.Active, Seats: 1, At: at.Add(time.Second)}
		if _, err := s.apply(t.Context(), "books", org.Slug, other, true); !errors.Is(err, errSecondSubscription) {
			t.Errorf("%v", err)
		}
		first.State, first.At = payments.Suspended, at.Add(time.Minute)
		s.apply(t.Context(), "ledger", org.Slug, first, true)
		if kept, err := s.apply(t.Context(), "books", org.Slug, other, true); !kept || err != nil {
			t.Errorf("a subscription that ended is replaced: %v %v", kept, err)
		}
		stale := payments.Subscription{Ref: "a", State: payments.Suspended, At: at.Add(time.Hour)}
		if kept, _ := s.apply(t.Context(), "ledger", org.Slug, stale, true); kept {
			t.Error("an answer that an old subscription has ended took the place of the one that pays")
		}
	})
}
