package dbkit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"modernc.org/sqlite"
)

const (
	maxAttempts = 6
	firstPause  = 5 * time.Millisecond
)

// Tx is a transaction. Like a DB it takes queries written with ? placeholders.
type Tx interface {
	Querier
}

type tx struct {
	*sql.Tx
	db *DB
}

func (t tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.Tx.QueryContext(ctx, t.db.Rebind(query), args...)
}

func (t tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.Tx.QueryRowContext(ctx, t.db.Rebind(query), args...)
}

func (t tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.Tx.ExecContext(ctx, t.db.Rebind(query), args...)
}

// InTx runs fn in a write transaction and commits it when fn returns nil. SQLite takes the write lock when
// the transaction begins. A transaction that fails because it lost a race (a serialization failure or
// deadlock on Postgres, a busy database on SQLite) is rolled back and run again, so fn must not leave effects
// outside the transaction.
func (db *DB) InTx(ctx context.Context, fn func(Tx) error) error {
	return db.retrying(ctx, db.w, nil, fn)
}

// InReadTx runs fn against one snapshot of the database, for a read that takes several queries. Nothing
// fn writes is kept.
func (db *DB) InReadTx(ctx context.Context, fn func(Tx) error) error {
	var opts *sql.TxOptions
	if db.dialect == Postgres {
		opts = &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead}
	}
	return db.retrying(ctx, db.r, opts, fn)
}

func (db *DB) retrying(ctx context.Context, pool *sql.DB, opts *sql.TxOptions, fn func(Tx) error) error {
	pause := firstPause
	for attempt := 1; ; attempt++ {
		err := db.once(ctx, pool, opts, fn)
		if err == nil || attempt == maxAttempts || !retryable(err) {
			return db.target.redact(err)
		}
		select {
		case <-ctx.Done():
			return db.target.redact(err)
		case <-time.After(pause):
		}
		pause *= 2
	}
}

func (db *DB) once(ctx context.Context, pool *sql.DB, opts *sql.TxOptions, fn func(Tx) error) error {
	raw, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("dbkit: begin: %w", err)
	}
	defer raw.Rollback()
	if err := fn(tx{raw, db}); err != nil {
		return err
	}
	if err := raw.Commit(); err != nil {
		return fmt.Errorf("dbkit: commit: %w", err)
	}
	return nil
}

// retryable is true of an error that a second run of the transaction can get past.
func retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code == "40001" || pg.Code == "40P01"
	}
	var lite *sqlite.Error
	if errors.As(err, &lite) {
		const busy, locked = 5, 6
		code := lite.Code() & 0xff
		return code == busy || code == locked
	}
	return false
}

// lock holds a lock named by key until the transaction ends. It does nothing on SQLite, whose write lock
// already serializes transactions.
func (db *DB) lock(ctx context.Context, t Tx, key string) error {
	if db.dialect != Postgres {
		return nil
	}
	h := fnv.New64a()
	h.Write([]byte("dbkit:" + key))
	_, err := t.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, int64(h.Sum64()))
	return err
}
