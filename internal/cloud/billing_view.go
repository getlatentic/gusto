package cloud

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/getlatentic/gusto/internal/cloud/payments"
)

// billingView is what the organization page shows of how the organization pays.
type billingView struct {
	Admin    bool
	State    payments.State
	Words    string
	Seats    int
	People   int
	Provider string
	// Offers are the ways an admin may subscribe, and empty when subscribing is not offered.
	Offers    []billingOffer
	Manage    bool
	Cancel    bool
	Proposals []Proposal
}

// billingOffer is one button that subscribes through one processor.
type billingOffer struct{ Provider, Words string }

// seatWords is a number of seats at a seat's price, as a person reads it.
func seatWords(seats int, price string) string {
	unit := "seats"
	if seats == 1 {
		unit = "seat"
	}
	return fmt.Sprintf("%d %s at %s", seats, unit, price)
}

// billingViewOf is the organization's billing as its page says it, and nil when this server does not bill or the
// organization is a demo.
func (s *Server) billingViewOf(ctx context.Context, org Org, person Person, people int) *billingView {
	if !s.Billing.Enabled() || org.Demo != nil {
		return nil
	}
	plan, held, err := s.Store.Plan(ctx, org.Slug)
	if err != nil {
		slog.Error("an organization's billing could not be read", "org", org.Slug, "error", err)
		return nil
	}
	view := &billingView{Admin: person.Role == Admin, Words: "Free trial", People: people}
	if held {
		s.describe(view, plan)
	}
	if view.Admin {
		view.Offers = s.offers(plan, held, people)
		view.Proposals, _ = s.Store.OpenProposals(org.Slug)
	}
	return view
}

// describe says what the plan is, and which of the processor's pages it may be sent to.
func (s *Server) describe(view *billingView, a Plan) {
	p, found := s.Billing.Providers.Get(a.Provider)
	if !found {
		return
	}
	view.State, view.Seats, view.Provider = a.State, a.Seats, p.Label()
	_, canceller := p.(payments.Canceller)
	_, portal := p.(payments.Portal)
	view.Cancel = canceller && a.State.Paying()
	view.Manage = portal && a.Customer != ""
	view.Words = planWords(a, p)
}

func planWords(a Plan, p payments.Provider) string {
	switch {
	case a.State == payments.Pending:
		return "Payment pending: waiting for the first payment at " + p.Label()
	case a.State == payments.PastDue:
		return "Payment overdue: " + p.Label() + " is trying again. " + seatWords(a.Seats, p.Price()) + " keep working meanwhile"
	case a.State == payments.Active:
		return "Paid: " + seatWords(a.Seats, p.Price())
	case a.PaidOnce:
		return "Ended: no new tasks or invitations start until it is renewed. Nothing has been deleted"
	}
	return "Free trial"
}

// offers are the processors an admin may subscribe through: none while the organization pays or a payment is awaited,
// unless the processor has an address to go back to.
func (s *Server) offers(a Plan, held bool, people int) []billingOffer {
	if held && (a.State.Paying() || (a.State == payments.Pending && a.ApproveURL == "")) {
		return nil
	}
	all := s.Billing.Providers.All()
	offers := make([]billingOffer, 0, len(all))
	for _, p := range all {
		words := "Subscribe"
		if len(all) > 1 {
			words += " with " + p.Label()
		}
		offers = append(offers, billingOffer{Provider: p.Name(), Words: words + ": " + seatWords(max(people, 1), p.Price())})
	}
	return offers
}
