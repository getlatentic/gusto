package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Stripe is the part of Stripe's API that bills seats: a Checkout page to subscribe, the
// customer portal for cards and invoices, and the quantity of a subscription. Only fields
// every API version has are read, so the account's own version is the one used.
type Stripe struct {
	// Key is the secret API key, Price the monthly price of one seat, and WebhookSecret
	// what Stripe signs the events it sends with.
	Key, Price, WebhookSecret string
	// SeatUSD is the seat's price in dollars a month, which the organization page shows.
	SeatUSD int
	// API is where the API is: https://api.stripe.com when empty. Tests point it elsewhere.
	API    string
	Client *http.Client
}

// stripeTimeout bounds every call to Stripe.
const stripeTimeout = 15 * time.Second

// Enabled says billing is set up.
func (s *Stripe) Enabled() bool {
	return s != nil && s.Key != "" && s.Price != "" && s.WebhookSecret != ""
}

// Checkout is the address of a Stripe page where an organization's admin subscribes for
// its seats. The organization's slug goes on the subscription, which is how its events
// find it again.
type Checkout struct {
	Org, Email, Customer string
	Seats                int
	Success, Cancel      string
}

// StartCheckout makes the Checkout page and says where it is.
func (s *Stripe) StartCheckout(ctx context.Context, c Checkout) (string, error) {
	form := url.Values{
		"mode":                             {"subscription"},
		"line_items[0][price]":             {s.Price},
		"line_items[0][quantity]":          {strconv.Itoa(max(c.Seats, 1))},
		"client_reference_id":              {c.Org},
		"subscription_data[metadata][org]": {c.Org},
		"success_url":                      {c.Success},
		"cancel_url":                       {c.Cancel},
	}
	if c.Customer != "" {
		form.Set("customer", c.Customer)
	} else {
		form.Set("customer_email", c.Email)
	}
	var made struct {
		URL string `json:"url"`
	}
	return made.URL, s.post(ctx, "/v1/checkout/sessions", form, &made)
}

// Portal is the address of Stripe's page where a customer changes their card, reads their
// invoices and ends the subscription.
func (s *Stripe) Portal(ctx context.Context, customer, back string) (string, error) {
	var made struct {
		URL string `json:"url"`
	}
	return made.URL, s.post(ctx, "/v1/billing_portal/sessions", url.Values{"customer": {customer}, "return_url": {back}}, &made)
}

// SetSeats sets how many seats a subscription bills for, and Stripe prorates the change.
func (s *Stripe) SetSeats(ctx context.Context, item string, seats int) error {
	form := url.Values{"quantity": {strconv.Itoa(seats)}, "proration_behavior": {"create_prorations"}}
	return s.post(ctx, "/v1/subscription_items/"+url.PathEscape(item), form, nil)
}

func (s *Stripe) post(ctx context.Context, path string, form url.Values, into any) error {
	ctx, cancel := context.WithTimeout(ctx, stripeTimeout)
	defer cancel()
	base := s.API
	if base == "" {
		base = "https://api.stripe.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.Key, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("stripe %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("stripe %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return stripeError(path, resp.StatusCode, body)
	}
	if into == nil {
		return nil
	}
	return json.Unmarshal(body, into)
}

// stripeError is what Stripe said was wrong, without the request, which carries no secret
// but is not wanted in a log either.
func stripeError(path string, status int, body []byte) error {
	var answer struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &answer) == nil && answer.Error.Message != "" {
		return fmt.Errorf("stripe %s: %d: %s", path, status, answer.Error.Message)
	}
	return fmt.Errorf("stripe %s: %d", path, status)
}

// errStripeSignature is every reason an event's signature is not believed.
var errStripeSignature = errors.New("the event is not signed by Stripe")
