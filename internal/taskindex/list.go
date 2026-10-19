package taskindex

import (
	"context"
	"database/sql"
	"strings"
)

func lower(s string) string { return strings.ToLower(s) }

// where is the WHERE clause that selects the rows a filter matches, with its arguments, or nothing at all
// for the empty filter.
func (s *store) where(f Filter) (string, []any) {
	conds, args := s.conditions(f)
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *store) conditions(f Filter) ([]string, []any) {
	var conds []string
	var args []any
	if len(f.States) > 0 {
		conds = append(conds, "state IN ("+strings.TrimSuffix(strings.Repeat("?,", len(f.States)), ",")+")")
		for _, state := range f.States {
			args = append(args, state)
		}
	}
	if !f.QueuedFrom.IsZero() {
		conds, args = append(conds, "queued >= ?"), append(args, f.QueuedFrom.Unix())
	}
	if !f.QueuedTo.IsZero() {
		conds, args = append(conds, "queued < ?"), append(args, f.QueuedTo.Unix())
	}
	for _, e := range []struct{ column, value string }{{"asked_lc", lower(f.Asked)}, {"team", f.Team}, {"repo", f.Repo},
		{"harness", f.AgentType}, {"instance", f.Instance}, {"thread", f.Thread}} {
		if e.value != "" {
			conds, args = append(conds, e.column+" = ?"), append(args, e.value)
		}
	}
	if f.Search != "" {
		conds, args = s.appendSearch(conds, args, f.Search)
	}
	return conds, args
}

func (s *store) appendSearch(conds []string, args []any, search string) ([]string, []any) {
	if strings.Contains(search, fieldSeparator) {
		return append(conds, "1 = 0"), args
	}
	return append(conds, s.contains), append(args, lower(search))
}

func (s *store) List(ctx context.Context, filter Filter, page Page) ([]Row, Cursor, error) {
	query, args, sort, err := s.listQuery(filter, page)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.rowsOf(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	limit := PageSize(page)
	if len(rows) <= limit {
		return rows, "", nil
	}
	rows = rows[:limit]
	return rows, sort.NewCursor(sort.PositionOf(rows[limit-1])), nil
}

// listQuery is the statement that reads a page, and one row more than the page holds to tell whether another follows.
func (s *store) listQuery(filter Filter, page Page) (string, []any, Sort, error) {
	sort, err := SortFor(page.Sort)
	if err != nil {
		return "", nil, "", err
	}
	conds, args := s.conditions(filter)
	if page.After != "" {
		after, err := sort.ParseCursor(page.After)
		if err != nil {
			return "", nil, "", err
		}
		conds, args = append(conds, afterCondition(sort)), append(args, afterArgs(after)...)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	query := `SELECT ` + selectColumns + ` FROM tasks` + where + ` ORDER BY ` + orderBy(sort) + ` LIMIT ?`
	return query, append(args, PageSize(page)+1), sort, nil
}

func orderBy(s Sort) string {
	return strings.Join(append(s.Columns(), "id"), " DESC, ") + " DESC"
}

func afterCondition(s Sort) string {
	cols := strings.Join(append(s.Columns(), "id"), ", ")
	return "(" + cols + ") < (" + strings.TrimSuffix(strings.Repeat("?, ", len(s.Columns())+1), ", ") + ")"
}

func afterArgs(p Position) []any {
	args := make([]any, 0, len(p.Key)+1)
	for _, k := range p.Key {
		args = append(args, k)
	}
	return append(args, p.ID)
}

func (s *store) Count(ctx context.Context, filter Filter) (map[string]int, error) {
	where, args := s.where(filter)
	counts := map[string]int{}
	err := s.query(ctx, `SELECT state, count(*) FROM tasks`+where+` GROUP BY state`, args, func(r *sql.Rows) error {
		var state string
		var n int
		if err := r.Scan(&state, &n); err != nil {
			return err
		}
		counts[state] = n
		return nil
	})
	return counts, err
}

func (s *store) AgentTypes(ctx context.Context, filter Filter) ([]string, error) {
	where, args := s.where(filter)
	var out []string
	err := s.query(ctx, `SELECT DISTINCT harness FROM tasks`+where+` ORDER BY harness`, args, func(r *sql.Rows) error {
		var name string
		if err := r.Scan(&name); err != nil {
			return err
		}
		out = append(out, name)
		return nil
	})
	return out, err
}
