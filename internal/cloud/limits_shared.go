package cloud

import (
	"context"
	"log/slog"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Limiter allows most events for a key in any window of per, counted in the database under the
// name scope, so that every front door sharing it is held to one count.
func (s *Store) Limiter(scope string, most int, per time.Duration) *Limiter {
	return &Limiter{most: most, per: per, store: s, scope: scope}
}

// allow counts an event for key and says whether it may go ahead. An event is refused, and not
// counted, when the key has had its most in the window. A database that cannot be asked refuses.
func (s *Store) allow(key string, most int, per time.Duration) bool {
	ctx := context.Background()
	allowed := false
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		now := s.now()
		if err := s.lock(ctx, tx, "rate:"+key); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rate_events WHERE key = ? AND until <= ?`, key, s.stamp(now)); err != nil {
			return err
		}
		held, err := count(ctx, tx, `SELECT count(*) FROM rate_events WHERE key = ?`, key)
		if err != nil {
			return err
		}
		if allowed = held < most; !allowed {
			return nil
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO rate_events (key, until) VALUES (?, ?)`, key, s.stamp(now.Add(per)))
		return err
	})
	if err != nil {
		slog.Error("a rate limit could not be counted, so the event is refused", "key", key, "error", err)
		return false
	}
	return allowed
}
