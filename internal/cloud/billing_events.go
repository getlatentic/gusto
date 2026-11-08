package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ErrBadSignature is a delivery PayPal says it did not send to this server's webhook.
var ErrBadSignature = errors.New("the webhook delivery is not signed by PayPal")

type webhookEvent struct {
	ID       string `json:"id"`
	Type     string `json:"event_type"`
	Resource struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"billing_agreement_id"`
	} `json:"resource"`
}

// subscription is the PayPal subscription an event is about: a subscription event is about its own resource,
// and a payment on a subscription names it as its billing agreement.
func (e webhookEvent) subscription() string {
	switch {
	case strings.HasPrefix(e.Type, "BILLING.SUBSCRIPTION."):
		return e.Resource.ID
	case strings.HasPrefix(e.Type, "PAYMENT.SALE."):
		return e.Resource.SubscriptionID
	}
	return ""
}

// HandleWebhook applies one delivery PayPal made to the webhook. It checks the signature with PayPal first and
// trusts nothing in the body before that. An event it has applied before, and one that is not about a subscription
// it knows, change nothing; an error other than ErrBadSignature is PayPal's cue to send the delivery again.
func (b *Billing) HandleWebhook(ctx context.Context, headers WebhookHeaders, body []byte) error {
	if !headers.complete() {
		return ErrBadSignature
	}
	genuine, err := b.PayPal.VerifyWebhook(ctx, headers, body)
	if err != nil {
		return err
	}
	if !genuine {
		return ErrBadSignature
	}
	var event webhookEvent
	if err := json.Unmarshal(body, &event); err != nil || event.ID == "" {
		return nil
	}
	if seen, err := b.Store.eventSeen(ctx, event.ID); err != nil || seen {
		return err
	}
	if id := event.subscription(); id != "" {
		if err := b.Reconcile(ctx, id); err != nil {
			return err
		}
	}
	return b.Store.markEvent(ctx, event.ID, event.Type)
}
