package cloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// stripeTolerance is how old a signed event may be, against replay.
const stripeTolerance = 5 * time.Minute

// verifyStripe says whether a Stripe-Signature header signs the body with the secret, and
// was made within the tolerance. Only v1 signatures count, against a downgrade.
func verifyStripe(header string, body []byte, secret string, now time.Time) bool {
	var stamp string
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		switch {
		case !found:
		case key == "t":
			stamp = value
		case key == "v1":
			signatures = append(signatures, value)
		}
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || len(signatures) == 0 {
		return false
	}
	if age := now.Sub(time.Unix(seconds, 0)); age > stripeTolerance || age < -stripeTolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "."))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, signature := range signatures {
		if hmac.Equal([]byte(signature), []byte(want)) {
			return true
		}
	}
	return false
}

// stripeEvent is the part of an event the seat billing reads.
type stripeEvent struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// stripeSubscription is the part of a subscription the seat billing reads.
type stripeSubscription struct {
	ID       string            `json:"id"`
	Customer string            `json:"customer"`
	Status   string            `json:"status"`
	Metadata map[string]string `json:"metadata"`
	Items    struct {
		Data []struct {
			ID       string `json:"id"`
			Quantity int    `json:"quantity"`
		} `json:"data"`
	} `json:"items"`
}

// subscriptionEvents are the events that say what a subscription now is.
var subscriptionEvents = map[string]bool{
	"customer.subscription.created": true,
	"customer.subscription.updated": true,
	"customer.subscription.deleted": true,
	"customer.subscription.paused":  true,
	"customer.subscription.resumed": true,
}

// billingOfEvent is the row a subscription event says an organization has, and false for an
// event that is not about a subscription or names no organization.
func billingOfEvent(event stripeEvent) (Billing, bool, error) {
	if !subscriptionEvents[event.Type] {
		return Billing{}, false, nil
	}
	var sub stripeSubscription
	if err := json.Unmarshal(event.Data.Object, &sub); err != nil {
		return Billing{}, false, err
	}
	org := sub.Metadata["org"]
	if org == "" || ValidSlug(org) != nil {
		return Billing{}, false, nil
	}
	b := Billing{Org: org, Customer: sub.Customer, Subscription: sub.ID, Status: sub.Status, EventAt: time.Unix(event.Created, 0).UTC()}
	if len(sub.Items.Data) > 0 {
		b.Item, b.Seats = sub.Items.Data[0].ID, sub.Items.Data[0].Quantity
	}
	return b, true, nil
}
