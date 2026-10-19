package taskindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/getlatentic/gusto/internal/dbkit"
)

var upsertTask = dbkit.Upsert{Table: "tasks", Columns: rowColumns, Conflict: []string{"id"}, Update: rowColumns[1:]}

// maxRowsPerTx is the most rows UpsertMany puts in one statement.
const maxRowsPerTx = 500

// store is the SQL implementation of Store.
type store struct {
	db *dbkit.DB
	// contains is true of a row whose search column holds the one argument.
	contains string
}

var _ Store = (*store)(nil)

func (s *store) Close() error { return s.db.Close() }

func (s *store) Upsert(ctx context.Context, row Row) error { return s.UpsertMany(ctx, []Row{row}) }

func (s *store) UpsertMany(ctx context.Context, rows []Row) error {
	rows = lastOfEach(rows)
	for start := 0; start < len(rows); start += maxRowsPerTx {
		if err := s.upsertChunk(ctx, rows[start:min(start+maxRowsPerTx, len(rows))]); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// lastOfEach drops all but the last row of any task named twice, since one statement may not change a row twice.
func lastOfEach(rows []Row) []Row {
	last := make(map[string]int, len(rows))
	for i, r := range rows {
		last[r.ID] = i
	}
	if len(last) == len(rows) {
		return rows
	}
	kept := make([]Row, 0, len(last))
	for i, r := range rows {
		if last[r.ID] == i {
			kept = append(kept, r)
		}
	}
	return kept
}

func (s *store) upsertChunk(ctx context.Context, rows []Row) error {
	args := make([]any, 0, len(rows)*len(rowColumns))
	for _, r := range rows {
		args = r.appendArgs(args)
	}
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		_, err := tx.ExecContext(ctx, upsertTask.SQL(len(rows)), args...)
		return err
	})
	if err != nil {
		return fmt.Errorf("taskindex: upsert: %w", err)
	}
	return nil
}

func (s *store) Delete(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("taskindex: delete: %w", err)
	}
	return nil
}

func (s *store) Get(ctx context.Context, id string) (Row, error) {
	return s.one(ctx, `SELECT `+selectColumns+` FROM tasks WHERE id = ?`, id)
}

func (s *store) FindKey(ctx context.Context, asked, key string) (Row, error) {
	if key == "" {
		return Row{}, ErrNotFound
	}
	rows, err := s.rowsOf(ctx, `SELECT `+selectColumns+` FROM tasks WHERE asked_lc = ? AND request_key = ? AND request_key <> ''`,
		lower(asked), key)
	if err != nil {
		return Row{}, err
	}
	for _, r := range rows {
		if r.Asked == asked {
			return r, nil
		}
	}
	return Row{}, ErrNotFound
}

func (s *store) one(ctx context.Context, query string, args ...any) (Row, error) {
	row, err := scanRow(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Row{}, ErrNotFound
	}
	if err != nil {
		return Row{}, fmt.Errorf("taskindex: %w", err)
	}
	return row, nil
}

func (s *store) rowsOf(ctx context.Context, query string, args ...any) ([]Row, error) {
	var out []Row
	err := s.query(ctx, query, args, func(r *sql.Rows) error {
		row, err := scanRow(r)
		out = append(out, row)
		return err
	})
	return out, err
}

// query runs a query and hands each row to scan.
func (s *store) query(ctx context.Context, query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("taskindex: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("taskindex: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("taskindex: %w", err)
	}
	return nil
}

func (s *store) Indexed(ctx context.Context) (map[string]Entry, error) {
	out := map[string]Entry{}
	err := s.query(ctx, `SELECT id, state, ended, seq, sum FROM tasks`, nil, func(r *sql.Rows) error {
		var id string
		var e Entry
		if err := r.Scan(&id, &e.State, &e.Ended, &e.Seq, &e.Sum); err != nil {
			return err
		}
		out[id] = e
		return nil
	})
	return out, err
}

// Reset deletes every row and says how many there were.
func (s *store) Reset(ctx context.Context) (int64, error) {
	done, err := s.db.ExecContext(ctx, `DELETE FROM tasks`)
	if err != nil {
		return 0, fmt.Errorf("taskindex: reset: %w", err)
	}
	return done.RowsAffected()
}
