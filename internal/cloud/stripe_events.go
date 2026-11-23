package cloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/payments"
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

// stripeStates is what each status of a Stripe subscription means to an organization. A status not here is not known.
var stripeStates = map[string]payments.State{
	"active": payments.Active, "trialing": payments.Active, "past_due": payments.PastDue, "incomplete": payments.Pending,
	"canceled": payments.Suspended, "unpaid": payments.Suspended, "incomplete_expired": payments.Suspended, "paused": payments.Suspended,
}

// subscriptionOfEvent is the subscription an event says, and false for an event that is not about one.
func subscriptionOfEvent(event stripeEvent) (payments.Subscription, bool, error) {
	if !subscriptionEvents[event.Type] {
		return payments.Subscription{}, false, nil
	}
	var sub stripeSubscription
	if err := json.Unmarshal(event.Data.Object, &sub); err != nil {
		return payments.Subscription{}, false, err
	}
	return subscriptionOfStripe(sub, time.Unix(event.Created, 0)), true, nil
}

// subscriptionOfStripe is a subscription as it was at a time. It names the organization the subscription was made
// for, which gusto put in its metadata, and none when there is no such name.
func subscriptionOfStripe(sub stripeSubscription, at time.Time) payments.Subscription {
	made := payments.Subscription{Ref: sub.ID, Customer: sub.Customer, State: stripeStates[sub.Status], At: at.UTC()}
	if org := sub.Metadata["org"]; ValidSlug(org) == nil {
		made.Org = org
	}
	if len(sub.Items.Data) > 0 {
		made.Handle, made.Seats = sub.Items.Data[0].ID, sub.Items.Data[0].Quantity
	}
	return made
}
