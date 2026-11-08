package cloud

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/quota"
)

// BillingState is where an organization stands with PayPal. An organization with no billing record is not
// billed at all.
type BillingState string

const (
	// BillingPending is a subscription the payer has not approved at PayPal yet. It changes nothing an organization may run.
	BillingPending BillingState = "pending"
	// BillingActive is a paid subscription.
	BillingActive BillingState = "active"
	// BillingPastDue is a subscription whose last payment failed. PayPal retries it, and the organization keeps running meanwhile.
	BillingPastDue BillingState = "past_due"
	// BillingSuspended is a subscription PayPal suspended or the payer cancelled. New tasks and invitations stop, and nothing is deleted.
	BillingSuspended BillingState = "suspended"
)

// BillingRecord is an organization's subscription as gusto keeps it. Seats is the quantity the payer approved.
type BillingRecord struct {
	Org            string
	State          BillingState
	SubscriptionID string
	ApproveURL     string
	Seats          int
	FailedPayments int
	SourceTime     time.Time
	Updated        time.Time
}

const billingColumns = `org, state, subscription_id, approve_url, seats, failed_payments, source_time, updated`

var billingFields = []string{"org", "state", "subscription_id", "approve_url", "seats", "failed_payments", "source_time", "updated"}

var putBilling = dbkit.Upsert{Table: "billing", Columns: billingFields, Conflict: []string{"org"}, Update: billingFields[1:]}

func scanBilling(r rowScanner) (BillingRecord, error) {
	var b BillingRecord
	var state string
	var source nullTime
	var updated dbkit.Time
	if err := r.Scan(&b.Org, &state, &b.SubscriptionID, &b.ApproveURL, &b.Seats, &b.FailedPayments, &source, &updated); err != nil {
		return BillingRecord{}, err
	}
	b.State, b.SourceTime, b.Updated = BillingState(state), timeOf(source), updated.Time
	return b, nil
}

func (s *Store) billingArgs(b BillingRecord) []any {
	return []any{b.Org, string(b.State), b.SubscriptionID, b.ApproveURL, b.Seats, b.FailedPayments, s.when(b.SourceTime), s.stamp(b.Updated)}
}

func fetchBilling(ctx context.Context, q dbkit.Querier, org string) (BillingRecord, bool, error) {
	return one(ctx, q, scanBilling, `SELECT `+billingColumns+` FROM billing WHERE org = ?`, org)
}

// BillingOf is an organization's subscription, and false when it has none.
func (s *Store) BillingOf(org string) (BillingRecord, bool, error) {
	return fetchBilling(context.Background(), s.db, org)
}

// billingBySubscription finds the organization a PayPal subscription was made for.
func (s *Store) billingBySubscription(id string) (BillingRecord, bool, error) {
	return one(context.Background(), s.db, scanBilling, `SELECT `+billingColumns+` FROM billing WHERE subscription_id = ?`, id)
}

// startSubscription records a subscription that waits for its payer, for an organization that has none or whose
// last one ended. A subscription that is pending, active or past due is left as it is.
func (s *Store) startSubscription(ctx context.Context, org, subscriptionID, approveURL string, seats int) error {
	return s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "billing:"+org); err != nil {
			return err
		}
		if have, found, err := fetchBilling(ctx, tx, org); err != nil {
			return err
		} else if found && have.State != BillingSuspended {
			return ErrSubscribed
		}
		record := BillingRecord{Org: org, State: BillingPending, SubscriptionID: subscriptionID, ApproveURL: approveURL, Seats: seats, Updated: s.now().UTC()}
		_, err := tx.ExecContext(ctx, putBilling.SQL(1), s.billingArgs(record)...)
		return err
	})
}

// Refusals billing can meet.
var (
	ErrSubscribed   = errors.New("this organization already has a subscription")
	ErrNoPayPal     = errors.New("payment by PayPal is not set up on this server")
	ErrNotBilled    = errors.New("this organization has no active subscription")
	ErrSeats        = errors.New("that number of seats is not one this organization can have")
	ErrProposal     = errors.New("there is no such proposal waiting for an answer")
	ErrProposalOpen = errors.New("a seat change is already waiting for an answer")
)

// A subscription's answer from PayPal is applied only if it is not older than the one applied last.
// It returns the record as it stands and the state the organization was in before.
func (s *Store) applySubscription(ctx context.Context, org string, answer Subscription, state BillingState) (was BillingState, record BillingRecord, err error) {
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "billing:"+org); err != nil {
			return err
		}
		have, found, err := fetchBilling(ctx, tx, org)
		if err != nil || !found || have.SubscriptionID != answer.ID {
			return err
		}
		was, record = have.State, have
		if !have.SourceTime.IsZero() && answer.UpdateTime.Before(have.SourceTime) {
			return nil
		}
		record.State, record.Seats, record.FailedPayments = state, answer.Seats(have.Seats), answer.FailedPayments
		record.SourceTime, record.Updated, record.ApproveURL = answer.UpdateTime, s.now().UTC(), ""
		if state == BillingPending {
			record.ApproveURL = have.ApproveURL
		}
		if _, err = tx.ExecContext(ctx, putBilling.SQL(1), s.billingArgs(record)...); err != nil {
			return err
		}
		return s.finishProposals(ctx, tx, org, record.Seats)
	})
	return was, record, err
}

// eventSeen says a webhook event was applied before; markEvent remembers that it was.
func (s *Store) eventSeen(ctx context.Context, id string) (bool, error) {
	n, err := count(ctx, s.db, `SELECT count(*) FROM billing_events WHERE id = ?`, id)
	return n > 0, err
}

var putEvent = dbkit.Upsert{Table: "billing_events", Columns: []string{"id", "kind", "received"}, Conflict: []string{"id"}}

func (s *Store) markEvent(ctx context.Context, id, kind string) error {
	_, err := s.db.ExecContext(ctx, putEvent.SQL(1), id, kind, s.stamp(s.now().UTC()))
	return err
}

// paidQuota is what an organization may run while it pays: the plan's quota, with a place for each seat.
// A suspended organization may start no task and take nobody in, and everything it made stays.
func paidQuota(plan quota.Quota, record BillingRecord) quota.Quota {
	q := plan
	q.Members, q.Computers, q.Invites = record.Seats, record.Seats, true
	if record.State == BillingSuspended {
		q.Jobs, q.Invites = 0, false
		q.Note = "Payment for this organization has stopped. An admin can subscribe again on the organization page."
	}
	return q
}

// newSecret is a random string a program holds; digest is what is kept of it.
func newSecret(prefix string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

var putToken = dbkit.Upsert{Table: "billing_tokens", Columns: []string{"org", "digest", "created"}, Conflict: []string{"org"}, Update: []string{"digest", "created"}}

// IssueAgentToken makes the bearer token an organization's agents ask about billing with. It replaces the old
// one, and is returned once: only its digest is kept.
func (s *Store) IssueAgentToken(org string) (string, error) {
	secret, err := newSecret("gbt_")
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(context.Background(), putToken.SQL(1), org, digest(secret), s.stamp(s.now().UTC()))
	return secret, err
}

// OrgOfAgentToken is the organization a bearer token was issued to.
func (s *Store) OrgOfAgentToken(secret string) (string, bool, error) {
	return one(context.Background(), s.db, func(r rowScanner) (string, error) {
		var org string
		return org, r.Scan(&org)
	}, `SELECT org FROM billing_tokens WHERE digest = ?`, digest(secret))
}
