package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// MostSeats is the most seats one organization may ask for.
const MostSeats = 500

// ProposalStatus is where a seat change stands. A proposal waits for an admin (pending), then for the payer at
// PayPal (awaiting_payer), and ends applied or declined.
type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "pending"
	ProposalAwaiting ProposalStatus = "awaiting_payer"
	ProposalApplied  ProposalStatus = "applied"
	ProposalDeclined ProposalStatus = "declined"
)

// Proposal is a change of seats that somebody, a person or an agent, asked for. It is never applied without an
// admin's answer and the payer's approval at PayPal.
type Proposal struct {
	ID         string
	Org        string
	Seats      int
	Reason     string
	Source     string
	Status     ProposalStatus
	ApproveURL string
	Created    time.Time
	DecidedBy  string
}

const proposalColumns = `id, org, seats, reason, source, status, approve_url, created, decided_by`

func scanProposal(r rowScanner) (Proposal, error) {
	var p Proposal
	var status string
	var created dbkit.Time
	if err := r.Scan(&p.ID, &p.Org, &p.Seats, &p.Reason, &p.Source, &status, &p.ApproveURL, &created, &p.DecidedBy); err != nil {
		return Proposal{}, err
	}
	p.Status, p.Created = ProposalStatus(status), created.Time
	return p, nil
}

const openProposals = `status IN ('pending', 'awaiting_payer')`

// OpenProposals are the seat changes of an organization that have not ended, oldest first.
func (s *Store) OpenProposals(org string) ([]Proposal, error) {
	return collect(context.Background(), s.db, scanProposal,
		`SELECT `+proposalColumns+` FROM seat_proposals WHERE org = ? AND `+openProposals+` ORDER BY created, id`, org)
}

// Propose records a request to change an organization's seats. It is refused unless the organization pays, the
// number is one it can have (at least its people, at most MostSeats, and not the number it has), and no other
// request is open.
func (s *Store) Propose(org string, seats int, reason, source string) (Proposal, error) {
	ctx := context.Background()
	var made Proposal
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "billing:"+org); err != nil {
			return err
		}
		record, found, err := fetchBilling(ctx, tx, org)
		if err != nil {
			return err
		}
		if !found || (record.State != BillingActive && record.State != BillingPastDue) {
			return ErrNotBilled
		}
		people, err := count(ctx, tx, `SELECT count(*) FROM people WHERE org = ?`, org)
		if err != nil {
			return err
		}
		if seats < max(people, 1) || seats > MostSeats || seats == record.Seats {
			return ErrSeats
		}
		if open, err := count(ctx, tx, `SELECT count(*) FROM seat_proposals WHERE org = ? AND `+openProposals, org); err != nil || open > 0 {
			if err == nil {
				err = ErrProposalOpen
			}
			return err
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		made = Proposal{ID: id, Org: org, Seats: seats, Reason: reason, Source: source, Status: ProposalPending, Created: s.now().UTC()}
		_, err = tx.ExecContext(ctx, `INSERT INTO seat_proposals (id, org, seats, reason, source, status, created) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			made.ID, org, seats, reason, source, string(made.Status), s.stamp(made.Created))
		return err
	})
	return made, err
}

func randomID() (string, error) {
	raw := make([]byte, 8)
	_, err := rand.Read(raw)
	return hex.EncodeToString(raw), err
}

// claimProposal moves a pending proposal to awaiting_payer for the admin who approved it, once: a second
// approval of the same proposal finds it gone.
func (s *Store) claimProposal(ctx context.Context, org, id, by string) (Proposal, error) {
	done, err := s.db.ExecContext(ctx, `UPDATE seat_proposals SET status = 'awaiting_payer', decided = ?, decided_by = ? WHERE id = ? AND org = ? AND status = 'pending'`,
		s.stamp(s.now().UTC()), by, id, org)
	if ok, err := inserted(done, err); err != nil || !ok {
		if err == nil {
			err = ErrProposal
		}
		return Proposal{}, err
	}
	p, _, err := one(ctx, s.db, scanProposal, `SELECT `+proposalColumns+` FROM seat_proposals WHERE id = ?`, id)
	return p, err
}

// releaseProposal puts a claimed proposal back to pending after PayPal refused the change, and keepApproval
// stores the address the payer approves it at.
func (s *Store) releaseProposal(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE seat_proposals SET status = 'pending', decided = NULL, decided_by = '' WHERE id = ? AND status = 'awaiting_payer'`, id)
	return err
}

func (s *Store) keepApproval(ctx context.Context, id, url string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE seat_proposals SET approve_url = ? WHERE id = ?`, url, id)
	return err
}

// Decline ends an open proposal without applying it.
func (s *Store) Decline(org, id, by string) error {
	done, err := s.db.ExecContext(context.Background(), `UPDATE seat_proposals SET status = 'declined', decided = ?, decided_by = ? WHERE id = ? AND org = ? AND `+openProposals,
		s.stamp(s.now().UTC()), by, id, org)
	if ok, err := inserted(done, err); err != nil || !ok {
		if err == nil {
			err = ErrProposal
		}
		return err
	}
	return nil
}

// finishProposals marks an organization's proposal applied when the subscription now has the seats it asked for.
func (s *Store) finishProposals(ctx context.Context, tx dbkit.Tx, org string, seats int) error {
	_, err := tx.ExecContext(ctx, `UPDATE seat_proposals SET status = 'applied' WHERE org = ? AND status = 'awaiting_payer' AND seats = ?`, org, seats)
	return err
}
