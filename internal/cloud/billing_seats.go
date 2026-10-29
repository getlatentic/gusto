package cloud

import (
	"context"
	"log/slog"
	"time"
)

// SeatSync keeps the seats Stripe bills each paying organization for equal to its members. It
// runs when woken after someone joins or is removed, and on a timer, which catches a change
// whose call to Stripe failed. Two front doors syncing at once set the same number twice.
type SeatSync struct {
	Store  *Store
	Stripe *Stripe
	wake   chan struct{}
}

// NewSeatSync makes the sync.
func NewSeatSync(store *Store, stripe *Stripe) *SeatSync {
	return &SeatSync{Store: store, Stripe: stripe, wake: make(chan struct{}, 1)}
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

// Once sets the seats of every paying organization whose members have changed, and says how
// many it set. A failure for one organization is logged and tried again next time.
func (y *SeatSync) Once(ctx context.Context) int {
	off, err := y.Store.seatsOff(ctx)
	if err != nil {
		slog.Error("the seats to bill could not be read", "error", err)
		return 0
	}
	set := 0
	for _, each := range off {
		if err := y.Stripe.SetSeats(ctx, each.item, each.want); err != nil {
			slog.Error("an organization's seats were not changed at Stripe", "org", each.org, "seats", each.want, "error", err)
			continue
		}
		if err := y.Store.SeatsBilled(each.org, each.want); err != nil {
			slog.Error("an organization's billed seats were not recorded", "org", each.org, "error", err)
			continue
		}
		set++
	}
	return set
}

// seatChange is a paying organization whose members are not the seats it is billed for.
type seatChange struct {
	org, item string
	want      int
}

// seatsOff is every paying organization billed for a number of seats other than its members.
func (s *Store) seatsOff(ctx context.Context) ([]seatChange, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.org, b.item, count(m.email) FROM billing b
		LEFT JOIN memberships m ON m.org = b.org
		WHERE b.item <> '' AND b.status IN ('active', 'trialing', 'past_due')
		GROUP BY b.org, b.item, b.seats HAVING count(m.email) <> b.seats AND count(m.email) > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var off []seatChange
	for rows.Next() {
		var each seatChange
		if err := rows.Scan(&each.org, &each.item, &each.want); err != nil {
			return nil, err
		}
		off = append(off, each)
	}
	return off, rows.Err()
}
