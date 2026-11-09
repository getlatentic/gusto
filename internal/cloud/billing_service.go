package cloud

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/url"

	"github.com/getlatentic/gusto/internal/cloud/payments"
)

// Billing sells gusto cloud by the seat through the processors it is given. The processor holds the payer and the
// card. gusto holds each organization's plan: which processor, which subscription, how many seats and in what state,
// and the organization's quota follows the plan. What differs between processors is only what a Provider says.
type Billing struct {
	Store     *Store
	Providers *payments.Registry
	// URL is the front door's address, which a payer is sent back to.
	URL *url.URL
}

// NewBilling bills through the providers, and is nil when there are none.
func NewBilling(store *Store, providers ...payments.Provider) *Billing {
	if len(providers) == 0 {
		return nil
	}
	return &Billing{Store: store, Providers: payments.NewRegistry(providers...)}
}

// Enabled says some processor is set up.
func (b *Billing) Enabled() bool { return b != nil && len(b.Providers.All()) > 0 }

// useURL gives the front door's address to billing that was made without one. It does nothing for no billing.
func (b *Billing) useURL(u *url.URL) {
	if b != nil && b.URL == nil {
		b.URL = u
	}
}

func (b *Billing) returnURL(org string) string {
	return b.URL.JoinPath("org", "billing", "return").String() + "?org=" + url.QueryEscape(org)
}

func (b *Billing) cancelURL(org string) string {
	return b.URL.JoinPath("org").String() + "?org=" + url.QueryEscape(org)
}

// provider is the processor with a name. With no name it is the only one, and with several it is refused.
func (b *Billing) provider(name string) (payments.Provider, error) {
	if name == "" && len(b.Providers.All()) == 1 {
		return b.Providers.All()[0], nil
	}
	p, found := b.Providers.Get(name)
	if !found {
		return nil, ErrNoProvider
	}
	return p, nil
}

// seatsFixed says the seats of a subscription at this processor are what its payer approved. A processor that sets
// its seats on gusto's word has seats that follow the people.
func seatsFixed(p payments.Provider) bool {
	_, follows := p.(payments.SeatSetter)
	return !follows
}

func subscriptionOf(a Plan) payments.Subscription {
	return payments.Subscription{Ref: a.Ref, Org: a.Org, State: a.State, Seats: a.Seats, Customer: a.Customer, Handle: a.Handle}
}

// planProvider is the processor an plan pays through.
func (b *Billing) planProvider(ctx context.Context, org string) (Plan, payments.Provider, error) {
	plan, found, err := b.Store.Plan(ctx, org)
	if err != nil || !found {
		return Plan{}, nil, errors.Join(err, ErrNotBilled)
	}
	p, found := b.Providers.Get(plan.Provider)
	if !found {
		return Plan{}, nil, ErrNoProvider
	}
	return plan, p, nil
}

// Subscribe starts a subscription for an organization, with a seat for each of its people, and returns the address its
// payer goes to. An organization that pays is refused: it is billed one way. While a subscription still waits for its payer,
// asking again returns its address and makes no second subscription, since a payer may be paying the first at that moment.
func (b *Billing) Subscribe(ctx context.Context, org, email, name string) (string, error) {
	p, err := b.provider(name)
	if err != nil {
		return "", err
	}
	plan, held, err := b.Store.Plan(ctx, org)
	if err != nil {
		return "", err
	}
	if held && plan.State.Paying() {
		return "", ErrSubscribed
	}
	if held && plan.State == payments.Pending && plan.Provider == p.Name() {
		if waiting, err := b.waitingApproval(ctx, p, plan); err != nil || waiting != "" {
			return waiting, err
		}
	}
	return b.start(ctx, p, org, email, plan, held)
}

func (b *Billing) start(ctx context.Context, p payments.Provider, org, email string, plan Plan, held bool) (string, error) {
	members, err := b.Store.Members(ctx, org)
	if err != nil {
		return "", err
	}
	request := payments.StartRequest{Org: org, Email: email, Seats: max(len(members), 1), Return: b.returnURL(org), Cancel: b.cancelURL(org)}
	if held && plan.Provider == p.Name() {
		request.Customer = plan.Customer
	}
	started, err := p.Start(ctx, request)
	if err != nil {
		return "", err
	}
	if started.Ref == "" {
		return started.URL, nil
	}
	err = b.Store.begin(ctx, Plan{Org: org, Provider: p.Name(), Ref: started.Ref, ApproveURL: started.URL, Seats: request.Seats, Fixed: seatsFixed(p)})
	return started.URL, err
}

// waitingApproval is the address a pending subscription is paid at, read from the processor because an address kept
// from earlier may have run out. It is empty when there is none to go to, and then a new subscription is started. A
// subscription that the processor says is no longer waiting is read in first, and is refused if it is paying.
func (b *Billing) waitingApproval(ctx context.Context, p payments.Provider, plan Plan) (string, error) {
	sub, err := p.Fetch(ctx, plan.Ref)
	if errors.Is(err, payments.ErrGone) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if sub.State == payments.Pending {
		return cmp.Or(sub.ApproveURL, plan.ApproveURL), nil
	}
	if err := b.reconcile(ctx, p, plan.Ref); err != nil {
		return "", err
	}
	if now, _, err := b.Store.Plan(ctx, plan.Org); err != nil || now.State != payments.Suspended {
		return "", errors.Join(err, ErrSubscribed)
	}
	return "", nil
}

// Return is where a payer lands after the processor. It reads the organization's own subscription from the processor
// and believes nothing in the address, so a payer who is back before the notice sees their state at once.
func (b *Billing) Return(ctx context.Context, org string) error {
	plan, found, err := b.Store.Plan(ctx, org)
	if err != nil || !found {
		return err
	}
	p, found := b.Providers.Get(plan.Provider)
	if !found {
		return nil
	}
	return b.reconcile(ctx, p, plan.Ref)
}

// Cancel ends an organization's subscription. Its people and work stay, and new tasks stop.
func (b *Billing) Cancel(ctx context.Context, org string) error {
	plan, p, err := b.planProvider(ctx, org)
	if err != nil {
		return err
	}
	canceller, found := p.(payments.Canceller)
	switch {
	case !plan.State.Paying():
		return ErrNotBilled
	case !found:
		return ErrUnsupported
	}
	if err := canceller.Cancel(ctx, subscriptionOf(plan), "Cancelled by an admin in gusto"); err != nil {
		return err
	}
	return b.reconcile(ctx, p, plan.Ref)
}

// Manage is the processor's page where the payer changes their card, reads their invoices and ends the subscription.
func (b *Billing) Manage(ctx context.Context, org string) (string, error) {
	plan, p, err := b.planProvider(ctx, org)
	if errors.Is(err, ErrNotBilled) {
		return "", errNoPortal
	}
	if err != nil {
		return "", err
	}
	portal, found := p.(payments.Portal)
	switch {
	case plan.Customer == "":
		return "", errNoPortal
	case !found:
		return "", ErrUnsupported
	}
	return portal.Portal(ctx, subscriptionOf(plan), b.cancelURL(org))
}

// Approve is an admin's yes to a seat change. The processor then needs the payer's own yes, at the address returned,
// and the seats change when it arrives. When the processor asks for nothing more the change is applied at once.
func (b *Billing) Approve(ctx context.Context, org, proposalID, by string) (string, error) {
	plan, p, err := b.planProvider(ctx, org)
	if err != nil {
		return "", err
	}
	reviser, found := p.(payments.SeatReviser)
	switch {
	case !plan.State.Paying():
		return "", ErrNotBilled
	case !found:
		return "", ErrUnsupported
	}
	proposal, err := b.Store.claimProposal(ctx, org, proposalID, by)
	if err != nil {
		return "", err
	}
	approveURL, err := reviser.ReviseSeats(ctx, subscriptionOf(plan), proposal.Seats, b.returnURL(org), b.cancelURL(org))
	if err != nil {
		return "", errors.Join(err, b.Store.releaseProposal(ctx, proposal.ID))
	}
	if err := b.Store.keepApproval(ctx, proposal.ID, approveURL); err != nil {
		return "", err
	}
	if approveURL == "" {
		return "", b.reconcile(ctx, p, plan.Ref)
	}
	return approveURL, nil
}

// Hook applies one delivery a processor made to its webhook. The processor checks it first, and nothing in the body is
// believed before that. A notice applied before, and one that is not about a subscription gusto knows, change nothing; an
// error other than payments.ErrBadSignature and payments.ErrMalformed is the processor's cue to send the delivery again.
func (b *Billing) Hook(ctx context.Context, name string, d payments.Delivery) error {
	p, found := b.Providers.Get(name)
	if !found {
		return ErrNoProvider
	}
	notice, err := p.Webhook(ctx, d)
	if err != nil {
		return err
	}
	if notice.ID != "" {
		if seen, err := b.Store.eventSeen(ctx, name, notice.ID); err != nil || seen {
			return err
		}
	}
	switch {
	case notice.Subscription != nil:
		err = b.accept(ctx, p, *notice.Subscription)
	case notice.Ref != "":
		err = b.reconcile(ctx, p, notice.Ref)
	}
	if err != nil || notice.ID == "" {
		return err
	}
	return b.Store.markEvent(ctx, name, notice.ID, notice.Kind)
}

// reconcile reads a subscription from its processor and makes the organization's plan say the same. It is a function of
// what the processor says now, so a repeat or a notice out of order changes nothing.
func (b *Billing) reconcile(ctx context.Context, p payments.Provider, ref string) error {
	sub, err := p.Fetch(ctx, ref)
	if err != nil {
		return err
	}
	return b.accept(ctx, p, sub)
}

// sameInstantTries is how many times an answer made at the instant of the one kept asks the processor again. One still
// tied is an error, and the processor sends the notice again at a later instant.
const sameInstantTries = 3

// accept keeps what a processor says of a subscription and writes the organization's quota. A subscription belongs to
// the organization whose plan holds its ref, or else to the one the processor says it was made for.
func (b *Billing) accept(ctx context.Context, p payments.Provider, sub payments.Subscription) error {
	org, found, err := b.orgOf(ctx, p, sub)
	if err != nil || !found || sub.State == "" {
		return err
	}
	kept, err := b.keep(ctx, p, org, sub)
	if kept {
		b.Store.quotaChanged(ctx, org)
	}
	return err
}

// keep stores an answer, and asks the processor again when the answer and the one kept were made at the same instant.
func (b *Billing) keep(ctx context.Context, p payments.Provider, org string, sub payments.Subscription) (bool, error) {
	for try := 0; ; try++ {
		kept, err := b.Store.apply(ctx, p.Name(), org, sub, seatsFixed(p))
		switch {
		case errors.Is(err, errSecondSubscription):
			slog.Error("an organization that pays subscribed again: refund the second subscription", "org", org, "provider", p.Name(), "subscription", sub.Ref)
			return false, nil
		case errors.Is(err, errSameInstant) && try < sameInstantTries:
			if sub, err = p.Fetch(ctx, sub.Ref); err != nil {
				return false, err
			}
			continue
		}
		return kept, err
	}
}

func (b *Billing) orgOf(ctx context.Context, p payments.Provider, sub payments.Subscription) (string, bool, error) {
	plan, found, err := b.Store.planOfRef(ctx, p.Name(), sub.Ref)
	if err != nil || found {
		return plan.Org, found, err
	}
	return sub.Org, sub.Org != "" && ValidSlug(sub.Org) == nil, nil
}
