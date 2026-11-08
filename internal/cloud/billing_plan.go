package cloud

import (
	"context"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/payments"
	"github.com/getlatentic/gusto/internal/dbkit"
)

// Plan is an organization's subscription as gusto keeps it, whichever processor bills it. An organization with no
// plan is not billed at all.
type Plan struct {
	Org      string
	Provider string
	Ref      string
	// Customer and Handle are the processor's names for the payer and for what its seat changes address.
	Customer, Handle string
	State            payments.State
	Seats            int
	// Fixed says the seats are what the payer approved, and cap the people. Otherwise they follow the people.
	Fixed          bool
	ApproveURL     string
	FailedPayments int
	// PaidOnce says the subscription was paying at some time, so one that ended unpaid changes no quota.
	PaidOnce   bool
	SourceTime time.Time
	Updated    time.Time
}

const planColumns = `org, provider, ref, customer, handle, state, seats, fixed_seats, approve_url, failed_payments, paid_once, source_time, updated`

var planFields = []string{"org", "provider", "ref", "customer", "handle", "state", "seats", "fixed_seats", "approve_url", "failed_payments", "paid_once", "source_time", "updated"}

var putPlan = dbkit.Upsert{Table: "billing_accounts", Columns: planFields, Conflict: []string{"org"}, Update: planFields[1:]}

func scanPlan(r rowScanner) (Plan, error) {
	var a Plan
	var state string
	var source nullTime
	var updated dbkit.Time
	if err := r.Scan(&a.Org, &a.Provider, &a.Ref, &a.Customer, &a.Handle, &state, &a.Seats, &a.Fixed, &a.ApproveURL, &a.FailedPayments, &a.PaidOnce, &source, &updated); err != nil {
		return Plan{}, err
	}
	a.State, a.SourceTime, a.Updated = payments.State(state), timeOf(source), updated.Time
	return a, nil
}

func (s *Store) planArgs(a Plan) []any {
	return []any{a.Org, a.Provider, a.Ref, a.Customer, a.Handle, string(a.State), a.Seats, a.Fixed, a.ApproveURL, a.FailedPayments, a.PaidOnce, s.when(a.SourceTime), s.stamp(a.Updated)}
}

func fetchPlan(ctx context.Context, q dbkit.Querier, org string) (Plan, bool, error) {
	return one(ctx, q, scanPlan, `SELECT `+planColumns+` FROM billing_accounts WHERE org = ?`, org)
}

// Plan is an organization's subscription, and false when it has none.
func (s *Store) Plan(ctx context.Context, org string) (Plan, bool, error) {
	return fetchPlan(ctx, s.db, org)
}

// planOfRef finds the plan a processor's subscription belongs to.
func (s *Store) planOfRef(ctx context.Context, provider, ref string) (Plan, bool, error) {
	return one(ctx, s.db, scanPlan, `SELECT `+planColumns+` FROM billing_accounts WHERE provider = ? AND ref = ?`, provider, ref)
}

// Refusals billing can meet.
var (
	ErrSubscribed   = errors.New("this organization already has a subscription")
	ErrNoProvider   = errors.New("this server does not take payment that way")
	ErrNotBilled    = errors.New("this organization has no active subscription")
	ErrSeats        = errors.New("that number of seats is not one this organization can have")
	ErrProposal     = errors.New("there is no such proposal waiting for an answer")
	ErrProposalOpen = errors.New("a seat change is already waiting for an answer")
	ErrFollows      = errors.New("this organization's seats follow its people, so there is nothing to propose")
	ErrUnsupported  = errors.New("the processor this organization pays through does not do that")

	// errNoPortal is a request for the processor's page of an organization that has none.
	errNoPortal = errors.New("this organization has no billing page")

	// errSecondSubscription is an organization that already pays being sent a second subscription, which happens when an
	// admin subscribes from two tabs. The first stands, and the operator refunds the second.
	errSecondSubscription = errors.New("an organization that pays was sent a second subscription")
	// errSameInstant is an answer made at the instant of the one kept and saying something else, so which is newer is
	// not known and the processor is asked again.
	errSameInstant = errors.New("two versions of a subscription were made at the same instant")
)

// begin records a subscription that waits for its payer, for an organization that has none or whose last one ended.
func (s *Store) begin(ctx context.Context, a Plan) error {
	return s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "billing:"+a.Org); err != nil {
			return err
		}
		if have, found, err := fetchPlan(ctx, tx, a.Org); err != nil {
			return err
		} else if found && have.State != payments.Suspended {
			return ErrSubscribed
		}
		a.State, a.Updated = payments.Pending, s.now().UTC()
		_, err := tx.ExecContext(ctx, putPlan.SQL(1), s.planArgs(a)...)
		return err
	})
}

// apply keeps what a processor says a subscription is, and marks the organization's quota due. It says whether the
// answer was kept. An answer older than the one kept changes nothing, since processors send them out of order, and neither
// does one for an organization that is gone or for a subscription that is not the one the plan holds and may not replace it.
func (s *Store) apply(ctx context.Context, provider, org string, sub payments.Subscription, fixed bool) (kept bool, err error) {
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "billing:"+org); err != nil {
			return err
		}
		if _, found, err := fetchOrg(ctx, tx, org); err != nil || !found {
			return err
		}
		have, held, err := fetchPlan(ctx, tx, org)
		if err != nil {
			return err
		}
		have.Org = org
		merged, replaced, err := merge(have, held, provider, sub, fixed, s.now().UTC())
		if err != nil || !replaced {
			return err
		}
		kept = true
		if _, err := tx.ExecContext(ctx, putPlan.SQL(1), s.planArgs(merged)...); err != nil {
			return err
		}
		if err := s.finishProposals(ctx, tx, org, merged.Seats); err != nil {
			return err
		}
		return markQuota(ctx, tx, org)
	})
	return kept, err
}

// merge is the plan an answer makes of the one held, and false when the answer changes nothing.
func merge(have Plan, held bool, provider string, sub payments.Subscription, fixed bool, now time.Time) (Plan, bool, error) {
	switch {
	case held && (have.Provider != provider || have.Ref != sub.Ref):
		if replaces, err := mayReplace(have, sub); err != nil || !replaces {
			return Plan{}, false, err
		}
		held = false
	case held:
		if stale, err := isStale(have, sub); err != nil || stale {
			return Plan{}, false, err
		}
	}
	return replace(have, held, provider, sub, fixed, now), true, nil
}

// mayReplace says an answer about another subscription takes the place of the one held: not while that one pays, and not
// by an answer that it has ended when the one held has not.
func mayReplace(have Plan, sub payments.Subscription) (bool, error) {
	switch {
	case have.State.Paying() && sub.State.Paying():
		return false, errSecondSubscription
	case have.State.Paying(), sub.State == payments.Suspended && have.State != payments.Suspended:
		return false, nil
	}
	return true, nil
}

// isStale says an answer about the subscription held is older than what is kept, or says the same of it. An answer made at the
// instant of the one kept that says something else is an error, since which is newer is not known.
func isStale(have Plan, sub payments.Subscription) (bool, error) {
	switch {
	case !have.SourceTime.IsZero() && sub.At.Before(have.SourceTime):
		return true, nil
	case sub.At.Equal(have.SourceTime) && sameAnswer(have, sub):
		return true, nil
	case sub.At.Equal(have.SourceTime):
		return false, errSameInstant
	}
	return false, nil
}

// sameAnswer says an answer says what the plan already says.
func sameAnswer(have Plan, sub payments.Subscription) bool {
	return have.State == sub.State && have.Seats == seatsOf(sub, have.Seats) && have.FailedPayments == sub.FailedPayments
}

// seatsOf is the seats an answer bills, and fallback when it does not say.
func seatsOf(sub payments.Subscription, fallback int) int {
	if sub.Seats > 0 {
		return sub.Seats
	}
	return fallback
}

// replace is the plan with an answer in it. A subscription that waits keeps the address it waits at.
func replace(have Plan, held bool, provider string, sub payments.Subscription, fixed bool, now time.Time) Plan {
	if !held {
		have = Plan{Org: have.Org, Provider: provider, Ref: sub.Ref}
	}
	have.Customer, have.Handle = keep(sub.Customer, have.Customer), keep(sub.Handle, have.Handle)
	have.State, have.Seats, have.Fixed, have.FailedPayments = sub.State, seatsOf(sub, have.Seats), fixed, sub.FailedPayments
	have.PaidOnce = (held && have.PaidOnce) || sub.State.Paying()
	have.SourceTime, have.Updated, have.ApproveURL = sub.At, now, ""
	if sub.State == payments.Pending {
		have.ApproveURL = keep(sub.ApproveURL, have.ApproveURL)
	}
	return have
}

func keep(answer, held string) string {
	if answer != "" {
		return answer
	}
	return held
}
