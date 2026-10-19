package usage

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// rowKey is a call's place in the order calls are listed in: the latest first, and from the greater task and
// line when calls were made in the same instant.
type rowKey struct {
	at   time.Time
	task string
	line int
}

func (r Row) key() rowKey { return rowKey{r.At, r.Task, r.Line} }

// Calls lists a session's calls that match the filter, the latest first.
func (s *store) Calls(ctx context.Context, session string, filter Filter, page Page) ([]Row, Cursor, error) {
	after, err := callsCursor(page.After)
	if err != nil {
		return nil, "", err
	}
	conds, args := s.conditions(filter)
	conds, args = append([]string{"session = ?"}, conds...), append([]any{session}, args...)
	limit := pageLimit(page)
	out, err := s.latest(ctx, conds, args, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	if len(out) == 0 {
		return nil, "", s.requireSession(ctx, session)
	}
	var next Cursor
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next = encodeCursor("calls", strconv.FormatInt(last.At.UnixNano(), 10), strconv.Itoa(last.Line), last.Task)
	}
	return out, next, nil
}

func callsCursor(c Cursor) (*rowKey, error) {
	if c == "" {
		return nil, nil
	}
	parts, err := decodeCursor(c, "calls", 3)
	if err != nil {
		return nil, err
	}
	ns, errAt := strconv.ParseInt(parts[0], 10, 64)
	line, errLine := strconv.Atoi(parts[1])
	if errAt != nil || errLine != nil {
		return nil, ErrBadCursor
	}
	return &rowKey{time.Unix(0, ns).UTC(), parts[2], line}, nil
}

// latest reads up to n calls that satisfy the conditions and come after the key in the listing order.
func (s *store) latest(ctx context.Context, conds []string, args []any, after *rowKey, n int) ([]Row, error) {
	conds, args = slices.Clone(conds), slices.Clone(args)
	if after != nil {
		conds = append(conds, "(called_at, task, line) < (?, ?, ?)")
		args = append(args, s.db.TimeArg(after.at), after.task, after.line)
	}
	query := `SELECT ` + selectColumns + ` FROM usage_calls WHERE ` + strings.Join(conds, " AND ") +
		` ORDER BY called_at DESC, task DESC, line DESC LIMIT ?`
	var out []Row
	err := s.query(ctx, s.db, query, append(args, n), func(r *sql.Rows) error {
		row, err := scanRow(r)
		out = append(out, row)
		return err
	})
	return out, err
}

// exportBatch is how many calls Export reads at a time.
const exportBatch = 1000

// Export streams the matching calls, the latest first. Each batch is its own short query that continues after
// the last call of the one before, so no transaction or cursor stays open while fn runs.
func (s *store) Export(ctx context.Context, filter Filter, fn func(Row) error) error {
	conds, args := s.conditions(filter)
	if len(conds) == 0 {
		conds = []string{"1 = 1"}
	}
	var after *rowKey
	for {
		batch, err := s.latest(ctx, conds, args, after, exportBatch)
		if err != nil {
			return err
		}
		for _, r := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(r); err != nil {
				return err
			}
		}
		if len(batch) < exportBatch {
			return ctx.Err()
		}
		k := batch[len(batch)-1].key()
		after = &k
	}
}

// requireSession is ErrNotFound unless the session has calls, for a page that came back empty.
func (s *store) requireSession(ctx context.Context, session string) error {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM usage_calls WHERE session = ? LIMIT 1`, session).Scan(&one)
	switch err {
	case nil:
		return nil
	case sql.ErrNoRows:
		return ErrNotFound
	}
	return fmt.Errorf("usage: calls: %w", err)
}
