package usage

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Sessions finds the page of sessions in one query that reads only the session and its rank, then adds up
// just those sessions in a few more, all in one snapshot so that a session is not described by calls its
// place in the page did not count.
func (s *store) Sessions(ctx context.Context, filter Filter, page Page) ([]Session, Cursor, error) {
	sortName, spec, err := sortFor(page.Sort)
	if err != nil {
		return nil, "", err
	}
	tx, err := s.r.BeginTx(ctx, s.d.readOnly)
	if err != nil {
		return nil, "", fmt.Errorf("usage: sessions: %w", err)
	}
	defer tx.Rollback()
	out, next, err := s.sessionPage(ctx, tx, filter, page, sortName, spec)
	if err != nil {
		return nil, "", err
	}
	if err := s.describe(ctx, tx, filter, out); err != nil {
		return nil, "", err
	}
	return out, next, nil
}

func (s *store) sessionPage(ctx context.Context, q querier, filter Filter, page Page, sortName string, spec sortSpec) ([]Session, Cursor, error) {
	conds, args := s.conditions(filter)
	where, group := "", "session"
	if len(conds) > 0 {
		where, group = " WHERE "+strings.Join(conds, " AND "), s.d.filteredGroup
	}
	having, args, err := s.sessionCursor(page.After, sortName, spec, args)
	if err != nil {
		return nil, "", err
	}
	limit := pageLimit(page)
	query := `SELECT session, ` + spec.expr + ` FROM usage_calls` + where + ` GROUP BY ` + group + having +
		` ORDER BY ` + spec.expr + ` DESC, session ASC LIMIT ?`
	var out []Session
	var ranks []rankCol
	err = s.query(ctx, q, query, append(args, limit+1), func(r *sql.Rows) error {
		var rank rankCol
		sess := Session{}
		if err := r.Scan(&sess.ID, &rank); err != nil {
			return err
		}
		out, ranks = append(out, sess), append(ranks, rank)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	var next Cursor
	if len(out) > limit {
		out = out[:limit]
		next = encodeCursor("sessions", sortName, strconv.FormatInt(int64(ranks[limit-1]), 10), out[limit-1].ID)
	}
	return out, next, nil
}

func (s *store) sessionCursor(after Cursor, sortName string, spec sortSpec, args []any) (string, []any, error) {
	if after == "" {
		return "", args, nil
	}
	parts, err := decodeCursor(after, "sessions", 3)
	if err != nil || parts[0] != sortName {
		return "", nil, ErrBadCursor
	}
	rank, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", nil, ErrBadCursor
	}
	v := spec.arg(s.d, rank)
	having := ` HAVING (` + spec.expr + ` < ? OR (` + spec.expr + ` = ? AND session > ?))`
	return having, append(args, v, v, parts[2]), nil
}

// describe fills in the sessions on a page with their totals, who made them, and what they used.
func (s *store) describe(ctx context.Context, q querier, filter Filter, sessions []Session) error {
	if len(sessions) == 0 {
		return nil
	}
	index := make(map[string]*Session, len(sessions))
	scope, args := s.scope(filter, sessions)
	for i := range sessions {
		index[sessions[i].ID] = &sessions[i]
	}
	steps := []func() error{
		func() error { return s.addTotals(ctx, q, scope, args, index) },
		func() error { return s.addWho(ctx, q, scope, args, index) },
		func() error {
			return s.addUsed(ctx, q, "provider", scope, args, index, func(x *Session) *[]string { return &x.Providers })
		},
		func() error {
			return s.addUsed(ctx, q, "model", scope, args, index, func(x *Session) *[]string { return &x.Models })
		},
		func() error {
			return s.addUsed(ctx, q, "task", scope, args, index, func(x *Session) *[]string { return &x.Tasks })
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// scope selects the calls of the listed sessions that match the filter.
func (s *store) scope(filter Filter, sessions []Session) (string, []any) {
	conds, args := s.conditions(filter)
	marks := make([]string, len(sessions))
	for i, sess := range sessions {
		marks[i] = "?"
		args = append(args, sess.ID)
	}
	conds = append(conds, "session IN ("+strings.Join(marks, ",")+")")
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *store) addTotals(ctx context.Context, q querier, scope string, args []any, index map[string]*Session) error {
	query := `SELECT session, MIN(called_at), MAX(called_at), ` + totalsColumns + ` FROM usage_calls` + scope + ` GROUP BY session`
	return s.query(ctx, q, query, args, func(r *sql.Rows) error {
		var id string
		var first, last timeCol
		var t Totals
		if err := r.Scan(append([]any{&id, &first, &last}, t.destinations()...)...); err != nil {
			return err
		}
		if sess := index[id]; sess != nil {
			sess.First, sess.Last, sess.Totals = first.Time, last.Time, t
		}
		return nil
	})
}

// addWho takes who made a session from its earliest matching call.
func (s *store) addWho(ctx context.Context, q querier, scope string, args []any, index map[string]*Session) error {
	query := `SELECT session, person, agent, engine FROM (
		SELECT session, person, agent, engine,
		       ROW_NUMBER() OVER (PARTITION BY session ORDER BY called_at, task, line) AS n
		FROM usage_calls` + scope + `) AS earliest WHERE n = 1`
	return s.query(ctx, q, query, args, func(r *sql.Rows) error {
		var id, person, agent, engine string
		if err := r.Scan(&id, &person, &agent, &engine); err != nil {
			return err
		}
		if sess := index[id]; sess != nil {
			sess.Person, sess.Agent, sess.Engine = person, agent, engine
		}
		return nil
	})
}

// addUsed lists each value of a column a session used, in the order of its first use and then by name.
func (s *store) addUsed(ctx context.Context, q querier, column, scope string, args []any, index map[string]*Session, into func(*Session) *[]string) error {
	query := `SELECT session, ` + column + ` FROM usage_calls` + scope +
		` GROUP BY session, ` + column + ` ORDER BY session, MIN(called_at), ` + column
	return s.query(ctx, q, query, args, func(r *sql.Rows) error {
		var id, value string
		if err := r.Scan(&id, &value); err != nil {
			return err
		}
		if sess := index[id]; sess != nil {
			list := into(sess)
			*list = append(*list, value)
		}
		return nil
	})
}
