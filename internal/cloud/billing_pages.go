package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// webhookBody is the most an event from Stripe may be.
const webhookBody = 512 << 10

// plan is what the organization page says of how the organization pays, and which of the
// two ways to Stripe it offers an admin.
type plan struct {
	Words     string
	Offer     string
	Subscribe bool
	Manage    bool
}

// seatWords is a number of seats at the seat's price, as a person reads it.
func seatWords(seats, usd int) string {
	unit := "seats"
	if seats == 1 {
		unit = "seat"
	}
	return fmt.Sprintf("%d %s at $%d a month each", seats, unit, usd)
}

// planOf is the organization's plan, and nil when this gusto cloud server does not bill or the
// organization is a demo.
func (s *Server) planOf(ctx context.Context, org Org, person Person) *plan {
	if !s.Stripe.Enabled() || org.Demo != nil {
		return nil
	}
	seats, _ := s.Store.SeatsWanted(ctx, org.Slug)
	billing, found, err := s.Store.Billing(ctx, org.Slug)
	if err != nil {
		slog.Error("an organization's billing could not be read", "org", org.Slug, "error", err)
		return nil
	}
	admin := person.Role == Admin
	p := &plan{Words: "Free trial", Offer: "Subscribe: " + seatWords(max(seats, 1), s.Stripe.SeatUSD), Subscribe: admin,
		Manage: admin && found && billing.Customer != ""}
	switch {
	case found && billing.Status == "incomplete":
		p.Words, p.Subscribe = "Payment pending: Stripe is waiting for the first payment", false
	case found && billing.Status == "past_due":
		p.Words, p.Subscribe = "Payment overdue: Stripe is trying the card again", false
	case found && billing.Paid():
		p.Words, p.Subscribe = "Paid: "+seatWords(billing.Seats, s.Stripe.SeatUSD), false
	case found && billing.Lapsed():
		p.Words = "Ended: no new tasks start until it is renewed"
	}
	return p
}

// subscribe sends an admin to Stripe to subscribe the organization for a seat for each member.
func (s *Server) subscribe(w http.ResponseWriter, r *http.Request, person Person) {
	org, found, err := s.Store.Org(r.Context(), person.Org)
	if !s.Stripe.Enabled() || err != nil || !found || org.Demo != nil {
		http.NotFound(w, r)
		return
	}
	if person.Role != Admin {
		s.drawOrg(r.Context(), w, http.StatusForbidden, person, sentence(ErrNotAdmin))
		return
	}
	billing, _, err := s.Store.Billing(r.Context(), org.Slug)
	if err != nil {
		s.drawOrg(r.Context(), w, http.StatusInternalServerError, person, sentence(err))
		return
	}
	if billing.Paid() {
		http.Redirect(w, r, "/org?org="+org.Slug, http.StatusSeeOther)
		return
	}
	seats, err := s.Store.SeatsWanted(r.Context(), org.Slug)
	if err != nil {
		s.drawOrg(r.Context(), w, http.StatusInternalServerError, person, sentence(err))
		return
	}
	back := s.URL.String() + "/org?org=" + url.QueryEscape(org.Slug)
	page, err := s.Stripe.StartCheckout(r.Context(), Checkout{Org: org.Slug, Email: person.Email, Customer: billing.Customer, Seats: seats,
		Success: back + "&billing=started", Cancel: back})
	if err != nil {
		slog.Error("a Stripe checkout was not started", "org", org.Slug, "error", err)
		s.drawOrg(r.Context(), w, http.StatusBadGateway, person, "Payments are not answering. Try again in a minute.")
		return
	}
	http.Redirect(w, r, page, http.StatusSeeOther)
}

// manageBilling sends an admin to Stripe's portal, where the card, the invoices and the
// subscription are.
func (s *Server) manageBilling(w http.ResponseWriter, r *http.Request, person Person) {
	billing, found, err := s.Store.Billing(r.Context(), person.Org)
	if !s.Stripe.Enabled() || err != nil || !found || billing.Customer == "" {
		http.NotFound(w, r)
		return
	}
	if person.Role != Admin {
		s.drawOrg(r.Context(), w, http.StatusForbidden, person, sentence(ErrNotAdmin))
		return
	}
	page, err := s.Stripe.Portal(r.Context(), billing.Customer, s.URL.String()+"/org?org="+url.QueryEscape(person.Org))
	if err != nil {
		slog.Error("a Stripe portal was not opened", "org", person.Org, "error", err)
		s.drawOrg(r.Context(), w, http.StatusBadGateway, person, "Payments are not answering. Try again in a minute.")
		return
	}
	http.Redirect(w, r, page, http.StatusSeeOther)
}

// stripeWebhook takes the events Stripe sends about subscriptions. An event that is not signed is
// refused; one that cannot be kept is answered with an error, so Stripe sends it again.
func (s *Server) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.Stripe.Enabled() {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookBody))
	if err != nil || !verifyStripe(r.Header.Get("Stripe-Signature"), body, s.Stripe.WebhookSecret, time.Now()) {
		http.Error(w, errStripeSignature.Error(), http.StatusBadRequest)
		return
	}
	var event stripeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "the event is not JSON", http.StatusBadRequest)
		return
	}
	billing, about, err := billingOfEvent(event)
	if err != nil {
		http.Error(w, "the subscription in the event cannot be read", http.StatusBadRequest)
		return
	}
	if about {
		err = s.keepSubscription(r.Context(), billing)
	}
	switch {
	case errors.Is(err, errSecondSubscription):
		slog.Error("an organization that pays subscribed again: refund the second subscription", "org", billing.Org, "subscription", billing.Subscription)
	case err != nil:
		slog.Error("a Stripe event was not kept", "event", event.ID, "error", err)
		http.Error(w, "the event was not kept", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("ok\n"))
}

// sameSecondTries is how many times an event made in the same second as the subscription kept asks Stripe for the
// subscription. One still tied is answered with an error, and Stripe sends it again in a later second.
const sameSecondTries = 3

// keepSubscription keeps what an event says of a subscription. When the event and the version kept were made in the
// same second, it keeps the subscription as Stripe says it is instead, by the same rule, which a later second decides.
func (s *Server) keepSubscription(ctx context.Context, b Billing) error {
	_, err := s.Store.RecordSubscription(ctx, b)
	for try := 0; try < sameSecondTries && errors.Is(err, errSameSecond); try++ {
		if b, err = s.Stripe.Subscription(ctx, b.Subscription); err == nil {
			_, err = s.Store.RecordSubscription(ctx, b)
		}
	}
	return err
}
