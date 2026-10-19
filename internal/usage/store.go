package usage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/getlatentic/gusto/internal/dbkit"
)

var insertCalls = dbkit.Upsert{Table: "usage_calls", Columns: rowColumns, Conflict: []string{"task", "line"}}

// maxRowsPerTx is the most rows Add puts in one transaction.
const maxRowsPerTx = 1000

// store is the SQL implementation of Store.
type store struct {
	db *dbkit.DB
	fl flavor
}

var _ Store = (*store)(nil)

func newStore(db *dbkit.DB) *store { return &store{db: db, fl: flavorOf(db.Dialect())} }

func (s *store) Close() error { return s.db.Close() }

// query runs a query and hands each row to scan.
func (s *store) query(ctx context.Context, q dbkit.Querier, query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("usage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("usage: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("usage: %w", err)
	}
	return nil
}

func (s *store) Add(ctx context.Context, rows []Row) error {
	for start := 0; start < len(rows); start += maxRowsPerTx {
		if err := s.addChunk(ctx, rows[start:min(start+maxRowsPerTx, len(rows))]); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (s *store) addChunk(ctx context.Context, rows []Row) error {
	args := make([]any, 0, len(rows)*len(rowColumns))
	for _, r := range rows {
		args = r.appendArgs(args, s.db)
	}
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		_, err := tx.ExecContext(ctx, insertCalls.SQL(len(rows)), args...)
		return err
	})
	if err != nil {
		return fmt.Errorf("usage: add: %w", err)
	}
	return nil
}

// Reset deletes every row and says how many there were.
func (s *store) Reset(ctx context.Context) (int64, error) {
	done, err := s.db.ExecContext(ctx, `DELETE FROM usage_calls`)
	if err != nil {
		return 0, fmt.Errorf("usage: reset: %w", err)
	}
	return done.RowsAffected()
}

func (s *store) Indexed(ctx context.Context) (map[string]int, error) {
	out := map[string]int{}
	err := s.query(ctx, s.db, `SELECT task, MAX(line) + 1 FROM usage_calls GROUP BY task`, nil, func(r *sql.Rows) error {
		var task string
		var n int
		if err := r.Scan(&task, &n); err != nil {
			return err
		}
		out[task] = n
		return nil
	})
	return out, err
}

const totalsColumns = `count(*), count(*) - count(usd),
	CAST(COALESCE(SUM(CASE WHEN refused <> '' THEN 1 ELSE 0 END), 0) AS BIGINT),
	CAST(COALESCE(SUM(input_tokens), 0) AS BIGINT),
	CAST(COALESCE(SUM(output_tokens), 0) AS BIGINT),
	CAST(COALESCE(SUM(cached_tokens), 0) AS BIGINT),
	CAST(COALESCE(SUM(usd), 0) AS DOUBLE PRECISION)`

func (t *Totals) destinations() []any {
	return []any{&t.Calls, &t.Unmeasured, &t.Refused, &t.InputTokens, &t.OutputTokens, &t.CachedTokens, &t.USD}
}

func (s *store) Totals(ctx context.Context, filter Filter) (Totals, error) {
	var t Totals
	where, args := s.where(filter)
	err := s.db.QueryRowContext(ctx, `SELECT `+totalsColumns+` FROM usage_calls`+where, args...).Scan(t.destinations()...)
	if err != nil {
		return Totals{}, fmt.Errorf("usage: totals: %w", err)
	}
	return t, nil
}
