package cloud

import (
	"context"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/quota"
)

// Billing is an organization's subscription, as Stripe last said it was. Seats is the
// quantity Stripe bills, which the seat sync keeps equal to the organization's members.
type Billing struct {
	Org          string
	Customer     string
	Subscription string
	Item         string
	Status       string
	Seats        int
	// EventAt is when Stripe made the newest event this row holds, so an older event
	// that arrives late changes nothing.
	EventAt time.Time
}

// Paid says the subscription is in good standing, or in the retries Stripe makes
// after a failed payment, which is the grace an organization has.
func (b Billing) Paid() bool {
	switch b.Status {
	case "active", "trialing", "past_due":
		return true
	}
	return false
}

// Lapsed says the organization subscribed and the subscription has ended or gone
// unpaid. It keeps its people and its work, and starts no new tasks.
func (b Billing) Lapsed() bool {
	switch b.Status {
	case "canceled", "unpaid", "incomplete_expired", "paused":
		return true
	}
	return false
}

// lapsedNote is what a person is told when a task is refused for a lapsed subscription.
const lapsedNote = "The organization's subscription has ended or its payment failed. An admin can renew it from the organization page."

const billingColumns = `org, customer, subscription, item, status, seats, event_at`

func scanBilling(r rowScanner) (Billing, error) {
	var b Billing
	var at dbkit.Time
	if err := r.Scan(&b.Org, &b.Customer, &b.Subscription, &b.Item, &b.Status, &b.Seats, &at); err != nil {
		return Billing{}, err
	}
	b.EventAt = at.Time
	return b, nil
}

var putBilling = dbkit.Upsert{Table: "billing", Columns: []string{"org", "customer", "subscription", "item", "status", "seats", "event_at"},
	Conflict: []string{"org"}, Update: []string{"customer", "subscription", "item", "status", "seats", "event_at"}}

func (s *Store) billingArgs(b Billing) []any {
	return []any{b.Org, b.Customer, b.Subscription, b.Item, b.Status, b.Seats, s.stamp(b.EventAt)}
}

func billingOf(ctx context.Context, q dbkit.Querier, slug string) (Billing, bool, error) {
	return one(ctx, q, scanBilling, `SELECT `+billingColumns+` FROM billing WHERE org = ?`, slug)
}

// Billing is an organization's subscription, and false when it has never had one.
func (s *Store) Billing(slug string) (Billing, bool, error) {
	return billingOf(context.Background(), s.db, slug)
}

// Billed is every organization with a subscription that bills seats.
func (s *Store) Billed() ([]Billing, error) {
	return collect(context.Background(), s.db, scanBilling, `SELECT `+billingColumns+` FROM billing WHERE item <> '' ORDER BY org`)
}

// SeatsWanted is how many seats an organization should be billed for: one for each member.
func (s *Store) SeatsWanted(slug string) (int, error) {
	return count(context.Background(), s.db, `SELECT count(*) FROM memberships WHERE org = ?`, slug)
}

// paidQuota is the quota of a paid organization: the paid limits, with tasks for each seat.
func (s *Store) paidQuota(seats int) quota.Quota {
	q := s.set.Limits.Paid
	q.Jobs *= max(seats, 1)
	return q
}

// lapsedQuota keeps a lapsed organization's people and computers and starts nothing.
func (s *Store) lapsedQuota() quota.Quota {
	q := s.set.Limits.Paid
	q.Jobs, q.Concurrent, q.Invites, q.Note = 0, 0, false, lapsedNote
	return q
}

// ApplyQuota writes the quota an organization has now: a demo's, a paid or lapsed
// subscription's, or, with none, the trial until one of its admins has confirmed their
// address and the verified limits after.
func (s *Store) ApplyQuota(slug string) error {
	ctx := context.Background()
	org, found, err := fetchOrg(ctx, s.db, slug)
	if err != nil || !found {
		return err
	}
	q, err := s.quotaOf(ctx, org)
	if err != nil {
		return err
	}
	return s.setQuota(slug, q)
}

func (s *Store) quotaOf(ctx context.Context, org Org) (quota.Quota, error) {
	if org.Demo != nil {
		return s.set.Demo.Limits, nil
	}
	billing, found, err := billingOf(ctx, s.db, org.Slug)
	if err != nil {
		return quota.Quota{}, err
	}
	switch {
	case found && billing.Paid():
		return s.paidQuota(billing.Seats), nil
	case found && billing.Lapsed():
		return s.lapsedQuota(), nil
	}
	verified, err := count(ctx, s.db, `SELECT count(*) FROM memberships m JOIN people p ON p.email = m.email
		WHERE m.org = ? AND m.role = ? AND p.verified IS NOT NULL`, org.Slug, string(Admin))
	if err != nil {
		return quota.Quota{}, err
	}
	if verified > 0 {
		return s.set.Limits.Verified, nil
	}
	return s.set.Limits.Trial, nil
}

// errSecondSubscription is an organization that already pays being sent a second subscription,
// which happens when an admin subscribes from two tabs. The first stands, and the operator
// refunds the second.
var errSecondSubscription = errors.New("an organization that pays was sent a second subscription")

// RecordSubscription keeps what a Stripe event says an organization's subscription is, and sets
// its quota to match. An event older than the one kept changes nothing, since Stripe may send
// them out of order, and so does one for an organization that is gone. It says whether the
// event was kept.
func (s *Store) RecordSubscription(b Billing) (bool, error) {
	ctx := context.Background()
	kept := false
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "org:"+b.Org); err != nil {
			return err
		}
		if _, found, err := fetchOrg(ctx, tx, b.Org); err != nil || !found {
			return err
		}
		held, found, err := billingOf(ctx, tx, b.Org)
		if err != nil {
			return err
		}
		if found && b.EventAt.Before(held.EventAt) {
			return nil
		}
		if found && held.Subscription != b.Subscription && held.Paid() {
			if b.Paid() {
				return errSecondSubscription
			}
			return nil
		}
		kept = true
		_, err = tx.ExecContext(ctx, putBilling.SQL(1), s.billingArgs(b)...)
		return err
	})
	if err != nil || !kept {
		return kept, err
	}
	return true, s.ApplyQuota(b.Org)
}

// SeatsBilled records that Stripe now bills an organization for seats, and sets its quota to match.
func (s *Store) SeatsBilled(slug string, seats int) error {
	if _, err := s.db.ExecContext(context.Background(), `UPDATE billing SET seats = ? WHERE org = ?`, seats, slug); err != nil {
		return err
	}
	return s.ApplyQuota(slug)
}
