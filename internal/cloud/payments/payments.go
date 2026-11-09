// Package payments is what gusto cloud asks of a payment processor or acquirer that bills an organization by the seat.
//
// A processor is one Provider: it starts a subscription, says what a subscription is now, and checks and reads the
// notices it sends. Everything else (what an organization may run, the seats it pays for, a seat change a person must
// approve, an agent that asks about billing) is gusto cloud's and is the same for every processor. A processor that can do
// more also has one of the optional interfaces, and the service uses what it finds.
//
// Nothing here knows of the database or of the pages, so a processor lives in its own package and is added by
// registering it.
package payments

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// State is where a subscription stands, whatever the processor calls it.
type State string

const (
	// Pending is a subscription nobody has paid for yet. It changes nothing an organization may run.
	Pending State = "pending"
	// Active is a subscription in good standing.
	Active State = "active"
	// PastDue is a subscription whose last payment failed and which the processor is retrying. The organization keeps running.
	PastDue State = "past_due"
	// Suspended is a subscription that has ended or has gone unpaid. New tasks and invitations stop, and nothing is deleted.
	Suspended State = "suspended"
)

// Paying says the organization has the seats it asked for: the subscription is active, or past due and in its retries.
func (s State) Paying() bool { return s == Active || s == PastDue }

// Subscription is what a processor says a subscription is, at the moment At.
type Subscription struct {
	// Ref is the processor's own name for the subscription, which is how its notices find it again.
	Ref string
	// Org is the organization the processor was told the subscription is for, and empty when it cannot say.
	Org string
	// State is empty for a status gusto does not know, and such an answer is ignored.
	State State
	// Seats is the quantity billed, and zero when the answer does not say.
	Seats int
	// Customer and Handle are the processor's names for the payer and for the thing whose quantity is changed. They are kept
	// and given back, and nothing else reads them.
	Customer, Handle string
	// ApproveURL is where a payer approves a subscription that is Pending, when the processor has such a place.
	ApproveURL     string
	FailedPayments int
	// At is when the processor made this answer, by its own clock. An answer older than the one kept changes nothing.
	At time.Time
}

// StartRequest is what a processor needs to start a subscription for an organization.
type StartRequest struct {
	Org, Email string
	Seats      int
	// Customer is the payer's name at the processor when the organization has paid before, and empty when it has not.
	Customer string
	// Return is where the payer lands after the processor's page, and Cancel where they land if they leave it.
	Return, Cancel string
}

// Started is where the payer goes, and the subscription's Ref when the processor has made one already. A processor that
// makes the subscription only when the payer pays leaves Ref empty, and its notice tells gusto the Ref later.
type Started struct {
	URL, Ref string
}

// Delivery is a notice a processor sent to its webhook, as received.
type Delivery struct {
	Header http.Header
	Body   []byte
}

// Notice is what a verified delivery says. An empty Notice is not about a subscription. A notice that carries the
// subscription gives it in Subscription; one that only names it gives its Ref, and the service reads it from the processor.
type Notice struct {
	// ID is the processor's name for the notice. A notice whose ID was applied before is not applied again.
	ID, Kind     string
	Ref          string
	Subscription *Subscription
}

// Provider is the part every processor has.
type Provider interface {
	// Name is the processor's short name: lower case, and part of the address of its webhook.
	Name() string
	// Label is its name as a person reads it.
	Label() string
	// Price is what one seat costs, as the organization page says it, such as "$25 a month each".
	Price() string
	// Origins are the addresses a person is sent to, which a page's forms may post to.
	Origins() []string
	Start(ctx context.Context, req StartRequest) (Started, error)
	// Fetch is the subscription as the processor says it is now.
	Fetch(ctx context.Context, ref string) (Subscription, error)
	// Webhook verifies a delivery and reads it. A delivery that is not the processor's returns ErrBadSignature; one that cannot be
	// read at all returns ErrMalformed. Nothing in a delivery is believed before it is verified.
	Webhook(ctx context.Context, d Delivery) (Notice, error)
}

// SeatSetter is a processor that changes the seats of a subscription on its own word. The seats then follow the
// organization's members.
type SeatSetter interface {
	SetSeats(ctx context.Context, sub Subscription, seats int) error
}

// SeatReviser is a processor that needs its payer's consent to change the seats. The change is a proposal an admin
// approves, and the payer then approves it at the address returned, which is empty when no consent was needed.
type SeatReviser interface {
	ReviseSeats(ctx context.Context, sub Subscription, seats int, ret, cancel string) (approveURL string, err error)
}

// Canceller is a processor through which gusto ends a subscription.
type Canceller interface {
	Cancel(ctx context.Context, sub Subscription, reason string) error
}

// Portal is a processor with a page where a payer changes their card, reads their invoices and ends the subscription.
type Portal interface {
	Portal(ctx context.Context, sub Subscription, back string) (string, error)
}

var (
	// ErrBadSignature is a delivery that the processor did not send to this server.
	ErrBadSignature = errors.New("the delivery is not signed by the processor")
	// ErrMalformed is a delivery that cannot be read.
	ErrMalformed = errors.New("the delivery cannot be read")
	// ErrGone is a subscription the processor no longer has, as with a checkout nobody finished.
	ErrGone = errors.New("the processor has no such subscription")
)

// Registry is the processors a gusto cloud server sells through, in the order they were added.
type Registry struct {
	byName map[string]Provider
	order  []Provider
}

// NewRegistry makes a registry of the providers. Two with one name are a mistake of the program that builds it.
func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{byName: map[string]Provider{}}
	for _, p := range providers {
		if _, taken := r.byName[p.Name()]; taken {
			panic("payments: two providers are named " + p.Name())
		}
		r.byName[p.Name()] = p
		r.order = append(r.order, p)
	}
	return r
}

// Get is the provider with a name.
func (r *Registry) Get(name string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	p, found := r.byName[name]
	return p, found
}

// All is the providers in the order they were added.
func (r *Registry) All() []Provider {
	if r == nil {
		return nil
	}
	return r.order
}
