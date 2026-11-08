package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/payments"
)

// stripeProvider is Stripe as a payments.Provider: Checkout to subscribe, the portal for cards and invoices, and seats that
// Stripe changes on gusto's word and prorates.
type stripeProvider struct{ stripe *Stripe }

// NewStripeProvider is the Stripe that bills seats, as a provider.
func NewStripeProvider(s *Stripe) payments.Provider { return stripeProvider{s} }

func (stripeProvider) Name() string  { return "stripe" }
func (stripeProvider) Label() string { return "Stripe" }
func (p stripeProvider) Price() string {
	return fmt.Sprintf("$%d a month each", p.stripe.SeatUSD)
}
func (stripeProvider) Origins() []string {
	return []string{"https://checkout.stripe.com", "https://billing.stripe.com"}
}

func (p stripeProvider) Start(ctx context.Context, req payments.StartRequest) (payments.Started, error) {
	page, err := p.stripe.StartCheckout(ctx, Checkout{Org: req.Org, Email: req.Email, Customer: req.Customer, Seats: req.Seats, Success: req.Return, Cancel: req.Cancel})
	return payments.Started{URL: page}, err
}

func (p stripeProvider) Fetch(ctx context.Context, ref string) (payments.Subscription, error) {
	return p.stripe.Subscription(ctx, ref)
}

func (p stripeProvider) Webhook(_ context.Context, d payments.Delivery) (payments.Notice, error) {
	if !verifyStripe(d.Header.Get("Stripe-Signature"), d.Body, p.stripe.WebhookSecret, time.Now()) {
		return payments.Notice{}, payments.ErrBadSignature
	}
	var event stripeEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		return payments.Notice{}, payments.ErrMalformed
	}
	sub, about, err := subscriptionOfEvent(event)
	if err != nil {
		return payments.Notice{}, payments.ErrMalformed
	}
	if !about {
		return payments.Notice{}, nil
	}
	return payments.Notice{ID: event.ID, Kind: event.Type, Subscription: &sub}, nil
}

func (p stripeProvider) SetSeats(ctx context.Context, sub payments.Subscription, seats int) error {
	return p.stripe.SetSeats(ctx, sub.Handle, seats)
}

func (p stripeProvider) Portal(ctx context.Context, sub payments.Subscription, back string) (string, error) {
	return p.stripe.Portal(ctx, sub.Customer, back)
}
