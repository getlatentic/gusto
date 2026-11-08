package cloud

import (
	"context"

	"github.com/getlatentic/gusto/internal/cloud/payments"
	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/quota"
)

// lapsedNote is what a person is told when a task is refused for a subscription that has ended.
const lapsedNote = "The organization's subscription has ended or its payment failed. An admin can renew it from the organization page."

// planQuota is what an organization may run by its subscription, and false when the subscription changes nothing:
// it waits for its payer, or it ended before anyone paid. A paying organization has a task allowance for each paid seat,
// and when its seats are what the payer approved it has a place for each of them. A suspended one may start no task
// and take nobody in, and everything it made stays.
func (s *Store) planQuota(a Plan) (quota.Quota, bool) {
	switch {
	case a.State.Paying():
		q := s.paidQuota(a.Seats)
		if a.Fixed {
			q.Members, q.Computers, q.Invites = a.Seats, a.Seats, true
		}
		return q, true
	case a.State == payments.Suspended && a.PaidOnce:
		return s.lapsedQuota(), true
	}
	return quota.Quota{}, false
}

// subscribedQuota is the quota of an organization whose subscription counts, and false when it has none.
func (s *Store) subscribedQuota(ctx context.Context, q dbkit.Querier, slug string) (quota.Quota, bool, error) {
	plan, found, err := fetchPlan(ctx, q, slug)
	if err != nil || !found {
		return quota.Quota{}, false, err
	}
	paid, counts := s.planQuota(plan)
	return paid, counts, nil
}

// lapsed says the organization subscribed and the subscription has ended.
func (s *Store) lapsed(ctx context.Context, slug string) bool {
	plan, found, err := s.Plan(ctx, slug)
	return err == nil && found && plan.State == payments.Suspended && plan.PaidOnce
}

// SeatsWanted is how many seats an organization should be billed for: one for each member.
func (s *Store) SeatsWanted(ctx context.Context, slug string) (int, error) {
	return count(ctx, s.db, `SELECT count(*) FROM memberships WHERE org = ?`, slug)
}
