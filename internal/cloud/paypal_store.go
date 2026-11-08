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

// PayPalState is where an organization stands with PayPal. An organization with no billing record is not
// billed at all.
type PayPalState string

const (
	// PayPalPending is a subscription the payer has not approved at PayPal yet. It changes nothing an organization may run.
	PayPalPending PayPalState = "pending"
	// PayPalActive is a paid subscription.
	PayPalActive PayPalState = "active"
	// PayPalPastDue is a subscription whose last payment failed. PayPal retries it, and the organization keeps running meanwhile.
	PayPalPastDue PayPalState = "past_due"
	// PayPalSuspended is a subscription PayPal suspended or the payer cancelled. New tasks and invitations stop, and nothing is deleted.
	PayPalSuspended PayPalState = "suspended"
)

// PayPalRecord is an organization's subscription as gusto keeps it. Seats is the quantity the payer approved.
type PayPalRecord struct {
	Org            string
	State          PayPalState
	SubscriptionID string
	ApproveURL     string
	Seats          int
	FailedPayments int
	// PaidOnce says the subscription was paying at some time, so one that ended unpaid changes no quota.
	PaidOnce   bool
	SourceTime time.Time
	Updated    time.Time
}

const payPalColumns = `org, state, subscription_id, approve_url, seats, failed_payments, paid_once, source_time, updated`

var payPalFields = []string{"org", "state", "subscription_id", "approve_url", "seats", "failed_payments", "paid_once", "source_time", "updated"}

var putPayPal = dbkit.Upsert{Table: "paypal_subscriptions", Columns: payPalFields, Conflict: []string{"org"}, Update: payPalFields[1:]}

func scanPayPal(r rowScanner) (PayPalRecord, error) {
	var b PayPalRecord
	var state string
	var source nullTime
	var updated dbkit.Time
	if err := r.Scan(&b.Org, &state, &b.SubscriptionID, &b.ApproveURL, &b.Seats, &b.FailedPayments, &b.PaidOnce, &source, &updated); err != nil {
		return PayPalRecord{}, err
	}
	b.State, b.SourceTime, b.Updated = PayPalState(state), timeOf(source), updated.Time
	return b, nil
}

func (s *Store) payPalArgs(b PayPalRecord) []any {
	return []any{b.Org, string(b.State), b.SubscriptionID, b.ApproveURL, b.Seats, b.FailedPayments, b.PaidOnce, s.when(b.SourceTime), s.stamp(b.Updated)}
}

func fetchPayPal(ctx context.Context, q dbkit.Querier, org string) (PayPalRecord, bool, error) {
	return one(ctx, q, scanPayPal, `SELECT `+payPalColumns+` FROM paypal_subscriptions WHERE org = ?`, org)
}

// PayPalOf is an organization's subscription, and false when it has none.
func (s *Store) PayPalOf(org string) (PayPalRecord, bool, error) {
	return fetchPayPal(context.Background(), s.db, org)
}

// payPalBySubscription finds the organization a PayPal subscription was made for.
func (s *Store) payPalBySubscription(id string) (PayPalRecord, bool, error) {
	return one(context.Background(), s.db, scanPayPal, `SELECT `+payPalColumns+` FROM paypal_subscriptions WHERE subscription_id = ?`, id)
}

// startSubscription records a subscription that waits for its payer, for an organization that has none or whose
// last one ended. A subscription that is pending, active or past due is left as it is.
func (s *Store) startSubscription(ctx context.Context, org, subscriptionID, approveURL string, seats int) error {
	return s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "paypal:"+org); err != nil {
			return err
		}
		if have, found, err := fetchPayPal(ctx, tx, org); err != nil {
			return err
		} else if found && have.State != PayPalSuspended {
			return ErrSubscribed
		}
		record := PayPalRecord{Org: org, State: PayPalPending, SubscriptionID: subscriptionID, ApproveURL: approveURL, Seats: seats, Updated: s.now().UTC()}
		_, err := tx.ExecContext(ctx, putPayPal.SQL(1), s.payPalArgs(record)...)
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
// It returns the record as it stands.
func (s *Store) applySubscription(ctx context.Context, org string, answer Subscription, state PayPalState) (record PayPalRecord, err error) {
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "paypal:"+org); err != nil {
			return err
		}
		have, found, err := fetchPayPal(ctx, tx, org)
		if err != nil || !found || have.SubscriptionID != answer.ID {
			return err
		}
		record = have
		if !have.SourceTime.IsZero() && answer.UpdateTime.Before(have.SourceTime) {
			return nil
		}
		record.State, record.Seats, record.FailedPayments = state, answer.Seats(have.Seats), answer.FailedPayments
		record.PaidOnce = have.PaidOnce || state == PayPalActive || state == PayPalPastDue
		record.SourceTime, record.Updated, record.ApproveURL = answer.UpdateTime, s.now().UTC(), ""
		if state == PayPalPending {
			record.ApproveURL = have.ApproveURL
		}
		if _, err = tx.ExecContext(ctx, putPayPal.SQL(1), s.payPalArgs(record)...); err != nil {
			return err
		}
		if err := s.finishProposals(ctx, tx, org, record.Seats); err != nil {
			return err
		}
		return markQuota(ctx, tx, org)
	})
	return record, err
}

// eventSeen says a webhook event was applied before; markEvent remembers that it was.
func (s *Store) eventSeen(ctx context.Context, id string) (bool, error) {
	n, err := count(ctx, s.db, `SELECT count(*) FROM paypal_events WHERE id = ?`, id)
	return n > 0, err
}

var putEvent = dbkit.Upsert{Table: "paypal_events", Columns: []string{"id", "kind", "received"}, Conflict: []string{"id"}}

func (s *Store) markEvent(ctx context.Context, id, kind string) error {
	_, err := s.db.ExecContext(ctx, putEvent.SQL(1), id, kind, s.stamp(s.now().UTC()))
	return err
}

// payPalQuota is what an organization may run by its PayPal subscription, and false when the subscription changes nothing:
// it waits for its payer, or it ended before anyone paid. A paying organization has a place for each paid seat. A
// suspended one may start no task and take nobody in, and everything it made stays.
func (s *Store) payPalQuota(record PayPalRecord) (quota.Quota, bool) {
	switch {
	case record.State == PayPalActive || record.State == PayPalPastDue:
		q := s.paidQuota(record.Seats)
		q.Members, q.Computers, q.Invites = record.Seats, record.Seats, true
		return q, true
	case record.State == PayPalSuspended && record.PaidOnce:
		return s.lapsedQuota(), true
	}
	return quota.Quota{}, false
}

// newSecret is a random string a program holds; digest is what is kept of it.
func newSecret(prefix string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

var putToken = dbkit.Upsert{Table: "paypal_agent_tokens", Columns: []string{"org", "digest", "created"}, Conflict: []string{"org"}, Update: []string{"digest", "created"}}

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
	}, `SELECT org FROM paypal_agent_tokens WHERE digest = ?`, digest(secret))
}
