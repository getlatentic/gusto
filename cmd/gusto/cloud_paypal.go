package main

import (
	"errors"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/settings"
)

// payPalSettings is how gusto cloud sells seats through PayPal's sandbox.
type payPalSettings struct {
	clientID, clientSecret, webhookID, planID string
}

func (p *payPalSettings) declare(set *settings.Set) {
	set.String(&p.clientID, "GUSTO_PAYPAL_CLIENT_ID", "", "the client id of a PayPal sandbox app that organizations pay for seats through; the client id, its secret and the webhook id are set together or not at all")
	set.Secret(&p.clientSecret, "GUSTO_PAYPAL_CLIENT_SECRET", "the secret of the PayPal sandbox app")
	set.String(&p.webhookID, "GUSTO_PAYPAL_WEBHOOK_ID", "", "the id of the PayPal sandbox webhook registered for <GUSTO_CLOUD_URL>/paypal/webhook")
	set.String(&p.planID, "GUSTO_PAYPAL_PLAN_ID", "", "the PayPal plan of a seat for a month. Without it the product and the plan are made once and remembered")
}

// billing is how gusto cloud sells by PayPal, and nil when it does not. Setting some of the three and not all is refused,
// so a gusto cloud server never takes payments it cannot hear about.
func (p payPalSettings) billing(store *cloud.Store) (*cloud.PayPalBilling, error) {
	config := cloud.PayPalConfig{ClientID: p.clientID, ClientSecret: p.clientSecret, WebhookID: p.webhookID}
	switch {
	case config.Ready():
		return &cloud.PayPalBilling{Store: store, PayPal: cloud.NewPayPalClient(config), PlanID: p.planID}, nil
	case config == (cloud.PayPalConfig{}):
		return nil, nil
	}
	return nil, errors.New("PayPal needs GUSTO_PAYPAL_CLIENT_ID, GUSTO_PAYPAL_CLIENT_SECRET and GUSTO_PAYPAL_WEBHOOK_ID together")
}
