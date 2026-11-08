package cloud

import (
	"context"
	"log/slog"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/payments"
	"github.com/getlatentic/gusto/internal/dbkit"
)

// SeatSync keeps the seats each paying organization is billed for equal to its members, for the processors that change
// seats on gusto's word. It runs when woken after someone joins or is removed, and on a timer, which catches a change
// whose call to the processor failed. Two gusto cloud servers syncing at once set the same number twice.
type SeatSync struct {
	Store     *Store
	Providers *payments.Registry
	wake      chan struct{}
}

// NewSeatSync makes the sync.
func NewSeatSync(store *Store, providers *payments.Registry) *SeatSync {
	return &SeatSync{Store: store, Providers: providers, wake: make(chan struct{}, 1)}
}

// Wake asks for a sync soon. It never waits.
func (y *SeatSync) Wake() {
	if y == nil {
		return
	}
	select {
	case y.wake <- struct{}{}:
	default:
	}
}

// Run syncs when woken and every interval, until the context ends.
func (y *SeatSync) Run(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		y.Once(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-y.wake:
		}
	}
}

// Once sets the seats of every organization whose seats follow its people and whose people have changed, and says how
// many it set. A failure for one organization is logged and tried again next time.
func (y *SeatSync) Once(ctx context.Context) int {
	off, err := y.Store.seatsOff(ctx)
	if err != nil {
		slog.Error("the seats to bill could not be read", "error", err)
		return 0
	}
	set := 0
	for _, each := range off {
		if y.set(ctx, each) {
			set++
		}
	}
	return set
}

func (y *SeatSync) set(ctx context.Context, each seatChange) bool {
	setter, found := y.setterOf(each.plan.Provider)
	if !found {
		return false
	}
	if err := setter.SetSeats(ctx, subscriptionOf(each.plan), each.want); err != nil {
		slog.Error("an organization's seats were not changed at its processor", "org", each.plan.Org, "provider", each.plan.Provider, "seats", each.want, "error", err)
		return false
	}
	if err := y.Store.SeatsBilled(ctx, each.plan.Org, each.want); err != nil {
		slog.Error("an organization's billed seats were not recorded", "org", each.plan.Org, "error", err)
		return false
	}
	return true
}

func (y *SeatSync) setterOf(provider string) (payments.SeatSetter, bool) {
	p, found := y.Providers.Get(provider)
	if !found {
		return nil, false
	}
	setter, found := p.(payments.SeatSetter)
	return setter, found
}

// seatChange is a paying organization whose people are not the seats it is billed for.
type seatChange struct {
	plan Plan
	want int
}

// seatsOff is every paying organization whose seats follow its people and are billed at a number other than them.
func (s *Store) seatsOff(ctx context.Context) ([]seatChange, error) {
	const people = `(SELECT count(*) FROM memberships m WHERE m.org = b.org)`
	accounts, err := collect(ctx, s.db, scanPlan, `SELECT `+planColumns+` FROM billing_accounts b
		WHERE NOT fixed_seats AND state IN ('active', 'past_due') AND `+people+` <> seats AND `+people+` > 0 ORDER BY org`)
	if err != nil {
		return nil, err
	}
	off := make([]seatChange, 0, len(accounts))
	for _, plan := range accounts {
		want, err := s.SeatsWanted(ctx, plan.Org)
		if err != nil {
			return nil, err
		}
		off = append(off, seatChange{plan: plan, want: want})
	}
	return off, nil
}

// SeatsBilled records that the processor now bills an organization for seats, and sets its quota to match. A notice the
// processor made before the change may still bring the old number, and the seat sync then sets the seats again.
func (s *Store) SeatsBilled(ctx context.Context, slug string, seats int) error {
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE billing_accounts SET seats = ? WHERE org = ?`, seats, slug); err != nil {
			return err
		}
		return markQuota(ctx, tx, slug)
	})
	if err != nil {
		return err
	}
	s.quotaChanged(ctx, slug)
	return nil
}
