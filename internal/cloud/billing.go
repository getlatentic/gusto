package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
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

// paidStatuses are a subscription in good standing, or in the retries Stripe makes after a failed payment, which is the
// grace an organization has.
var paidStatuses = []string{"active", "trialing", "past_due"}

// Paid says the subscription is one of the paidStatuses.
func (b Billing) Paid() bool { return slices.Contains(paidStatuses, b.Status) }

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

// errSecondSubscription is an organization that already pays being sent a second subscription,
// which happens when an admin subscribes from two tabs. The first stands, and the operator
// refunds the second.
var errSecondSubscription = errors.New("an organization that pays was sent a second subscription")

// errSameSecond is an event about the subscription kept, made in the same second as the one kept and saying something
// else. Stripe stamps events in whole seconds, so which is newer is not known: the subscription is asked of Stripe.
var errSameSecond = errors.New("two versions of a subscription were made in the same second")

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
		if found && held.Subscription == b.Subscription && b.EventAt.Equal(held.EventAt) {
			if held.sameAs(b) {
				return nil
			}
			return errSameSecond
		}
		if found && held.Subscription != b.Subscription && held.Paid() {
			if b.Paid() {
				return errSecondSubscription
			}
			return nil
		}
		kept = true
		if _, err := tx.ExecContext(ctx, putBilling.SQL(1), s.billingArgs(b)...); err != nil {
			return err
		}
		return markQuota(ctx, tx, b.Org)
	})
	if err != nil || !kept {
		return kept, err
	}
	s.quotaChanged(b.Org)
	return true, nil
}

// sameAs says two versions of a subscription say the same of it.
func (b Billing) sameAs(other Billing) bool {
	return b.Customer == other.Customer && b.Item == other.Item && b.Status == other.Status && b.Seats == other.Seats
}

// SeatsBilled records that Stripe now bills an organization for seats, and sets its quota to match. An event Stripe made
// before the change may still bring the old number, and the seat sync then sets the seats again.
func (s *Store) SeatsBilled(slug string, seats int) error {
	ctx := context.Background()
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE billing SET seats = ? WHERE org = ?`, seats, slug); err != nil {
			return err
		}
		return markQuota(ctx, tx, slug)
	})
	if err != nil {
		return err
	}
	s.quotaChanged(slug)
	return nil
}

// CreditUsed is what an organization has used of its credit, as its server last wrote it, and nothing before it has used any.
func (s *Store) CreditUsed(slug string) float64 {
	text, err := os.ReadFile(filepath.Join(s.DataDir(slug), "credit-used.json"))
	if err != nil {
		return 0
	}
	var used struct {
		USD float64 `json:"usd"`
	}
	if json.Unmarshal(text, &used) != nil {
		return 0
	}
	return used.USD
}
