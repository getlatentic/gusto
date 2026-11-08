package main

import (
	"errors"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/cloud"
)

// billingFromEnvironment turns on payment by PayPal's sandbox when its credentials are in the environment:
// GUSTO_PAYPAL_CLIENT_ID and GUSTO_PAYPAL_CLIENT_SECRET of a sandbox app, and GUSTO_PAYPAL_WEBHOOK_ID of the webhook
// registered for this front door's /billing/paypal/webhook. GUSTO_PAYPAL_PLAN_ID names an existing plan; without it
// the plan is made once. None set means no billing, and some set is a mistake worth stopping for.
func billingFromEnvironment(store *cloud.Store) (*cloud.Billing, error) {
	cfg := cloud.PayPalConfig{
		ClientID:     strings.TrimSpace(os.Getenv("GUSTO_PAYPAL_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("GUSTO_PAYPAL_CLIENT_SECRET")),
		WebhookID:    strings.TrimSpace(os.Getenv("GUSTO_PAYPAL_WEBHOOK_ID")),
	}
	switch {
	case cfg == (cloud.PayPalConfig{}):
		return nil, nil
	case !cfg.Ready():
		return nil, errors.New("PayPal needs GUSTO_PAYPAL_CLIENT_ID, GUSTO_PAYPAL_CLIENT_SECRET and GUSTO_PAYPAL_WEBHOOK_ID together")
	}
	return &cloud.Billing{Store: store, PayPal: cloud.NewPayPalClient(cfg), PlanID: strings.TrimSpace(os.Getenv("GUSTO_PAYPAL_PLAN_ID"))}, nil
}
