package main

import (
	"errors"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/cloud/payments"
	"github.com/getlatentic/gusto/internal/settings"
)

// payPalSettings is how gusto cloud sells seats through PayPal's sandbox.
type payPalSettings struct {
	clientID, clientSecret, webhookID, planID string
}

func (p *payPalSettings) declare(set *settings.Set) {
	set.String(&p.clientID, "GUSTO_PAYPAL_CLIENT_ID", "", "the client id of a PayPal sandbox app that organizations pay for seats through; the client id, its secret and the webhook id are set together or not at all")
	set.Secret(&p.clientSecret, "GUSTO_PAYPAL_CLIENT_SECRET", "the secret of the PayPal sandbox app")
	set.String(&p.webhookID, "GUSTO_PAYPAL_WEBHOOK_ID", "", "the id of the PayPal sandbox webhook registered for <GUSTO_CLOUD_URL>/billing/webhook/paypal")
	set.String(&p.planID, "GUSTO_PAYPAL_PLAN_ID", "", "the PayPal plan of a seat for a month. Without it the product and the plan are made once and remembered")
}

// client is the PayPal sandbox client gusto cloud sells through, and nil when it does not sell by PayPal. Setting some of the
// three and not all is refused, so a gusto cloud server never takes payments it cannot hear about.
func (p payPalSettings) client() (*payPalSale, error) {
	config := cloud.PayPalConfig{ClientID: p.clientID, ClientSecret: p.clientSecret, WebhookID: p.webhookID}
	switch {
	case config.Ready():
		return &payPalSale{client: cloud.NewPayPalClient(config), planID: p.planID}, nil
	case config == (cloud.PayPalConfig{}):
		return nil, nil
	}
	return nil, errors.New("PayPal needs GUSTO_PAYPAL_CLIENT_ID, GUSTO_PAYPAL_CLIENT_SECRET and GUSTO_PAYPAL_WEBHOOK_ID together")
}

// payPalSale is a PayPal set up to sell, waiting for the accounts it keeps its plan in.
type payPalSale struct {
	client *cloud.PayPalClient
	planID string
}

// billingOf is the billing of the processors that are set up, in the order organizations are offered them, and nil when
// none is. A processor added to gusto cloud is added here.
func billingOf(store *cloud.Store, stripe *cloud.Stripe, payPal *payPalSale) *cloud.Billing {
	var providers []payments.Provider
	if stripe != nil {
		providers = append(providers, cloud.NewStripeProvider(stripe))
	}
	if payPal != nil {
		providers = append(providers, cloud.NewPayPalProvider(store, payPal.client, payPal.planID))
	}
	return cloud.NewBilling(store, providers...)
}
