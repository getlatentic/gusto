package cloud

import (
	"encoding/json"
	"strings"

	"github.com/getlatentic/gusto/internal/cloud/payments"
)

type payPalEvent struct {
	ID       string `json:"id"`
	Type     string `json:"event_type"`
	Resource struct {
		ID             string `json:"id"`
		SubscriptionID string `json:"billing_agreement_id"`
	} `json:"resource"`
}

// subscription is the PayPal subscription an event is about, and empty for one about something else.
func (e payPalEvent) subscription() string {
	switch {
	case strings.HasPrefix(e.Type, "BILLING.SUBSCRIPTION."):
		return e.Resource.ID
	case strings.HasPrefix(e.Type, "PAYMENT.SALE."):
		return e.Resource.SubscriptionID
	}
	return ""
}

// readPayPalEvent is the notice a verified delivery is. A body that is not an event with an id is not about anything.
func readPayPalEvent(body []byte) payments.Notice {
	var event payPalEvent
	if json.Unmarshal(body, &event) != nil || event.ID == "" {
		return payments.Notice{}
	}
	return payments.Notice{ID: event.ID, Kind: event.Type, Ref: event.subscription()}
}
