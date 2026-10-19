package cloud

import (
	"context"
	"database/sql"
	"embed"
	"hash/fnv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

const databaseFile = "cloud.db"

//go:embed migrations
var migrationFiles embed.FS

var migrations = dbkit.Migrations{Scope: "cloud", Table: "cloud_schema_migrations", FS: migrationFiles, Dir: "migrations"}

const (
	openAttempts = 40
	openPause    = 50 * time.Millisecond
)

// openDatabase opens the database. Front doors started together on a SQLite file that does not exist yet each
// ask for the write-ahead log at once, and all but one are told the file is busy, so a busy answer is asked again.
func openDatabase(ctx context.Context, url, root string) (*dbkit.DB, error) {
	for attempt := 1; ; attempt++ {
		db, err := dbkit.Open(ctx, url, root, databaseFile)
		if err == nil || attempt == openAttempts || !strings.Contains(err.Error(), "SQLITE_BUSY") {
			return db, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(openPause):
		}
	}
}

type rowScanner interface{ Scan(dest ...any) error }

// when is a time as a query argument, and NULL for the zero time.
func (s *Store) when(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return s.db.TimeArg(t)
}

func (s *Store) stamp(t time.Time) any { return s.db.TimeArg(t) }

type nullTime = sql.Null[dbkit.Time]

func timeOf(n nullTime) time.Time {
	if n.Valid {
		return n.V.Time
	}
	return time.Time{}
}

// lock holds a lock named key until the transaction ends. It is what makes a check and the write that
// depends on it one step for every front door on a Postgres database; SQLite's write lock already does.
func (s *Store) lock(ctx context.Context, tx dbkit.Tx, key string) error {
	if s.db.Dialect() != dbkit.Postgres {
		return nil
	}
	h := fnv.New64a()
	h.Write([]byte("gusto-cloud:" + key))
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(?)`, int64(h.Sum64()))
	return err
}

// collect reads every row of a query.
func collect[T any](ctx context.Context, q dbkit.Querier, scan func(rowScanner) (T, error), query string, args ...any) ([]T, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// one reads the first row of a query, and says whether there was one.
func one[T any](ctx context.Context, q dbkit.Querier, scan func(rowScanner) (T, error), query string, args ...any) (T, bool, error) {
	item, err := scan(q.QueryRowContext(ctx, query, args...))
	if err == sql.ErrNoRows {
		var none T
		return none, false, nil
	}
	return item, err == nil, err
}

// count is the single number a query returns.
func count(ctx context.Context, q dbkit.Querier, query string, args ...any) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

// inserted says a statement added a row. An insert that names its conflict and does nothing adds none.
func inserted(done sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := done.RowsAffected()
	return n > 0, err
}
