package payments

import (
	"context"
	"testing"
)

type named string

func (n named) Name() string                                         { return string(n) }
func (n named) Label() string                                        { return string(n) }
func (n named) Price() string                                        { return "" }
func (n named) Origins() []string                                    { return nil }
func (n named) Start(context.Context, StartRequest) (Started, error) { return Started{}, nil }
func (n named) Fetch(context.Context, string) (Subscription, error)  { return Subscription{}, nil }
func (n named) Webhook(context.Context, Delivery) (Notice, error)    { return Notice{}, nil }

func TestARegistryKeepsTheOrderProvidersWereAddedAndFindsThemByName(t *testing.T) {
	r := NewRegistry(named("stripe"), named("paypal"))
	if got := r.All(); len(got) != 2 || got[0].Name() != "stripe" || got[1].Name() != "paypal" {
		t.Errorf("%v", got)
	}
	if _, found := r.Get("paypal"); !found {
		t.Error("paypal is registered")
	}
	if _, found := r.Get("adyen"); found {
		t.Error("adyen is not")
	}
	var none *Registry
	if _, found := none.Get("stripe"); found || len(none.All()) != 0 {
		t.Error("no registry sells through nobody")
	}
}

func TestTwoProvidersWithOneNameAreRefused(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("the second was taken")
		}
	}()
	NewRegistry(named("stripe"), named("stripe"))
}

func TestOnlyActiveAndPastDueAreSubscriptionsThatPay(t *testing.T) {
	for state, want := range map[State]bool{Active: true, PastDue: true, Pending: false, Suspended: false, "": false} {
		if state.Paying() != want {
			t.Errorf("%q", state)
		}
	}
}
