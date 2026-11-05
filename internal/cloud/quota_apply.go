package cloud

import (
	"context"
	"log/slog"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/quota"
)

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

// markQuota marks an organization's quota due to be written, in the transaction that changed what it is worked out from,
// so a quota the change's writer does not write is written by the sweep.
func markQuota(ctx context.Context, tx dbkit.Tx, slug string) error {
	_, err := tx.ExecContext(ctx, `UPDATE orgs SET quota_due = TRUE WHERE slug = ?`, slug)
	return err
}

// quotaChanged writes the quota of an organization whose change has committed. One not written stays marked, and the
// sweep writes it.
func (s *Store) quotaChanged(ctx context.Context, slug string) {
	if err := s.ApplyQuota(ctx, slug); err != nil {
		slog.Error("an organization's quota was not written, and the sweep writes it", "org", slug, "error", err)
	}
}

// ApplyQuota writes the quota an organization has now: a demo's, a paid or lapsed
// subscription's, or, with none, the trial until one of its admins has confirmed their
// address and the verified limits after. It clears the organization's mark first, which holds
// its row until the file is written: a second ApplyQuota waits, so an older quota is never
// written over a newer one, and a change that marks the row again waits too, so its mark stays
// for the sweep. The row is held across the write of one small file.
func (s *Store) ApplyQuota(ctx context.Context, slug string) error {

	return s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE orgs SET quota_due = FALSE WHERE slug = ?`, slug); err != nil {
			return err
		}
		org, found, err := fetchOrg(ctx, tx, slug)
		if err != nil || !found {
			return err
		}
		q, err := s.quotaOf(ctx, tx, org)
		if err != nil {
			return err
		}
		return s.setQuota(slug, q)
	})
}

// ApplyQuotas writes every organization's quota again, so a change to the limits a release or the operator sets reaches the
// organizations made before it. One that cannot be written is logged and the rest are written.
func (s *Store) ApplyQuotas(ctx context.Context) error {
	orgs, err := s.Orgs(ctx)
	if err != nil {
		return err
	}
	for _, org := range orgs {
		s.quotaChanged(ctx, org.Slug)
	}
	return nil
}

// dueBatch is how many marked quotas one sweep writes; a sweep that finds more writes the rest next time.
const dueBatch = 200

// RunQuotas writes the marked quotas every interval, until the context ends.
func (s *Store) RunQuotas(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if err := s.applyDueQuotas(ctx); err != nil {
			slog.Error("the quotas due could not be read", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) applyDueQuotas(ctx context.Context) error {
	due, err := collect(ctx, s.db, scanSlug, `SELECT slug FROM orgs WHERE quota_due ORDER BY slug LIMIT ?`, dueBatch)
	if err != nil {
		return err
	}
	for _, slug := range due {
		s.quotaChanged(ctx, slug)
	}
	return nil
}

func scanSlug(r rowScanner) (string, error) {
	var slug string
	return slug, r.Scan(&slug)
}

func (s *Store) quotaOf(ctx context.Context, q dbkit.Querier, org Org) (quota.Quota, error) {
	if org.Demo != nil {
		return s.set.Demo.Limits, nil
	}
	billing, found, err := billingOf(ctx, q, org.Slug)
	if err != nil {
		return quota.Quota{}, err
	}
	switch {
	case found && billing.Paid():
		return s.paidQuota(billing.Seats), nil
	case found && billing.Lapsed():
		return s.lapsedQuota(), nil
	}
	verified, err := count(ctx, q, `SELECT count(*) FROM memberships m JOIN people p ON p.email = m.email
		WHERE m.org = ? AND m.role = ? AND p.verified IS NOT NULL`, org.Slug, string(Admin))
	if err != nil {
		return quota.Quota{}, err
	}
	if verified > 0 {
		return s.set.Limits.Verified, nil
	}
	return s.set.Limits.Trial, nil
}
