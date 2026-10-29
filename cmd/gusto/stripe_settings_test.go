package main

import "testing"

func TestBillingIsOffWithNoStripeSettingsAndRefusedWithSome(t *testing.T) {
	for _, name := range []string{"GUSTO_CLOUD_STRIPE_KEY", "GUSTO_CLOUD_STRIPE_PRICE", "GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET"} {
		t.Setenv(name, "")
	}
	if s, err := stripeSettings(); s != nil || err != nil {
		t.Errorf("%v %v", s, err)
	}
	t.Setenv("GUSTO_CLOUD_STRIPE_KEY", "sk_test")
	if _, err := stripeSettings(); err == nil {
		t.Error("a key with no price and no webhook secret was taken")
	}
	t.Setenv("GUSTO_CLOUD_STRIPE_PRICE", "price_seat")
	t.Setenv("GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET", "whsec")
	if s, err := stripeSettings(); err != nil || s.SeatUSD != 25 {
		t.Errorf("%+v %v", s, err)
	}
}
