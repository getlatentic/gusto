package cloud

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/cloud/payments"
)

const settingPayPalPlan = "paypal_plan_id"

// payPalProvider is PayPal's sandbox as a payments.Provider: PayPal Subscriptions, whose seat changes need the payer's own
// consent at PayPal. It reads a subscription from PayPal after every delivery and trusts nothing the delivery says of it.
type payPalProvider struct {
	client *PayPalClient
	store  *Store
	// plan is the PayPal plan to subscribe to. When it is empty the plan is made at PayPal once and remembered.
	plan string
}

// NewPayPalProvider is the PayPal sandbox as a provider. planID may be empty.
func NewPayPalProvider(store *Store, client *PayPalClient, planID string) payments.Provider {
	return &payPalProvider{client: client, store: store, plan: planID}
}

func (*payPalProvider) Name() string  { return "paypal" }
func (*payPalProvider) Label() string { return "PayPal" }
func (*payPalProvider) Price() string {
	return "$" + strings.TrimSuffix(SeatPrice, ".00") + " a month each"
}
func (*payPalProvider) Origins() []string { return []string{"https://www.sandbox.paypal.com"} }

func (p *payPalProvider) planID(ctx context.Context) (string, error) {
	if p.plan != "" {
		return p.plan, nil
	}
	if id, found, err := p.store.setting(ctx, settingPayPalPlan); err != nil || found {
		return id, err
	}
	id, err := p.client.EnsurePlan(ctx)
	if err != nil {
		return "", err
	}
	return id, p.store.saveSetting(ctx, settingPayPalPlan, id)
}

// Start makes the subscription at PayPal, whose request id is new each time, so a repeat of the call is a new subscription.
func (p *payPalProvider) Start(ctx context.Context, req payments.StartRequest) (payments.Started, error) {
	plan, err := p.planID(ctx)
	if err != nil {
		return payments.Started{}, err
	}
	nonce, err := randomID()
	if err != nil {
		return payments.Started{}, err
	}
	made, err := p.client.CreateSubscription(ctx, plan, req.Seats, req.Org, req.Return, req.Cancel, "gusto-"+req.Org+"-"+nonce)
	if err == nil && made.ApproveURL == "" {
		err = errors.New("paypal gave no address to approve the subscription at")
	}
	return payments.Started{URL: made.ApproveURL, Ref: made.ID}, err
}

// Fetch is PayPal's current record of a subscription. It names no organization: PayPal's subscriptions are found by the
// ref that Start returned.
func (p *payPalProvider) Fetch(ctx context.Context, ref string) (payments.Subscription, error) {
	answer, err := p.client.Subscription(ctx, ref)
	var refusal *PayPalError
	if errors.As(err, &refusal) && refusal.Status == http.StatusNotFound {
		return payments.Subscription{}, payments.ErrGone
	}
	if err != nil {
		return payments.Subscription{}, err
	}
	return payments.Subscription{Ref: answer.ID, State: payPalState(answer), Seats: answer.Seats(0), ApproveURL: answer.ApproveURL,
		FailedPayments: answer.FailedPayments, At: answer.UpdateTime}, nil
}

// payPalState is what a PayPal subscription means for the organization, and empty for a status not known.
func payPalState(s PayPalSubscription) payments.State {
	switch s.Status {
	case "APPROVAL_PENDING", "APPROVED":
		return payments.Pending
	case "ACTIVE":
		if s.FailedPayments > 0 {
			return payments.PastDue
		}
		return payments.Active
	case "SUSPENDED", "CANCELLED", "EXPIRED":
		return payments.Suspended
	}
	return ""
}

// Webhook asks PayPal whether the delivery is one it sent to this server's webhook, and takes the subscription it is about:
// a subscription event is about its own resource, and a payment on a subscription names it as its billing agreement.
func (p *payPalProvider) Webhook(ctx context.Context, d payments.Delivery) (payments.Notice, error) {
	headers := webhookHeaders(d.Header)
	if !headers.complete() {
		return payments.Notice{}, payments.ErrMalformed
	}
	genuine, err := p.client.VerifyWebhook(ctx, headers, d.Body)
	if err != nil {
		return payments.Notice{}, err
	}
	if !genuine {
		return payments.Notice{}, payments.ErrBadSignature
	}
	return readPayPalEvent(d.Body), nil
}

func (p *payPalProvider) ReviseSeats(ctx context.Context, sub payments.Subscription, seats int, ret, cancel string) (string, error) {
	return p.client.ReviseSeats(ctx, sub.Ref, seats, ret, cancel)
}

func (p *payPalProvider) Cancel(ctx context.Context, sub payments.Subscription, reason string) error {
	return p.client.Cancel(ctx, sub.Ref, reason)
}
