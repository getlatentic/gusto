package cloud

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// ErrNotLeading is a write refused because another front door has taken the lease since this one took it.
var ErrNotLeading = errors.New("this front door no longer leads")

// controllerLease names the one lease there is.
const controllerLease = "controller"

// Lead takes or keeps the lease that makes one front door the one that runs the controller, until ttl from now. It gives
// the lease's epoch when holder has it, and 0 when another holder has it and it has not run out. A holder that takes the
// lease from another is given the next epoch, so that what the earlier holder writes after is refused.
func (s *Store) Lead(ctx context.Context, holder string, ttl time.Duration) (int64, error) {

	var epoch int64
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "leader"); err != nil {
			return err
		}
		var held string
		var until dbkit.Time
		now := s.now()
		err := tx.QueryRowContext(ctx, `SELECT holder, epoch, until FROM leader WHERE name = ?`, controllerLease).Scan(&held, &epoch, &until)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			epoch = 1
			_, err := tx.ExecContext(ctx, `INSERT INTO leader (name, holder, epoch, until) VALUES (?, ?, ?, ?)`,
				controllerLease, holder, epoch, s.stamp(now.Add(ttl)))
			return err
		case err != nil:
			return err
		case held != holder && now.Before(until.Time):
			epoch = 0
			return nil
		case held != holder:
			epoch++
		}
		_, err = tx.ExecContext(ctx, `UPDATE leader SET holder = ?, epoch = ?, until = ? WHERE name = ?`,
			holder, epoch, s.stamp(now.Add(ttl)), controllerLease)
		return err
	})
	if err != nil {
		return 0, err
	}
	return epoch, nil
}

// Resign gives up the lease, when holder has it, so another front door takes it at once rather than when it runs out.
func (s *Store) Resign(ctx context.Context, holder string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE leader SET until = ? WHERE name = ? AND holder = ?`,
		s.stamp(time.Unix(0, 0)), controllerLease, holder)
	return err
}

// SetRuntimeAt is SetRuntime for the front door that leads at epoch, and is refused with ErrNotLeading when the lease has
// moved on. The check and the write are one step for every front door. An epoch of 0 is a controller no lease governs.
func (s *Store) SetRuntimeAt(ctx context.Context, epoch int64, slug string, rt Runtime) error {
	switch {
	case epoch == 0:
		return s.SetRuntime(ctx, slug, rt)
	case epoch < 0:
		return ErrNotLeading
	}
	rt.Updated = s.now().UTC().Truncate(time.Second)

	return s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "leader"); err != nil {
			return err
		}
		var current int64
		if err := tx.QueryRowContext(ctx, `SELECT epoch FROM leader WHERE name = ?`, controllerLease).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotLeading
			}
			return err
		}
		if current != epoch {
			return ErrNotLeading
		}
		_, err := tx.ExecContext(ctx, putRuntime.SQL(1),
			slug, string(rt.State), rt.Cell, rt.Port, rt.ProxyPort, rt.Error, rt.Restarts, s.stamp(rt.Updated), rt.Command)
		return err
	})
}

// Unprovisioned is every organization that has no runtime yet: one signed up on a front door that does not lead, which the
// leader provisions.
func (s *Store) Unprovisioned(ctx context.Context) ([]string, error) {
	return collect(ctx, s.db, func(r rowScanner) (string, error) {
		var slug string
		return slug, r.Scan(&slug)
	}, `SELECT o.slug FROM orgs o LEFT JOIN runtime r ON r.slug = o.slug WHERE r.slug IS NULL ORDER BY o.slug`)
}
