package usage_test

import (
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

// rankedSession is a session the sort tests build: how many calls it has, when its last was, what it spent and
// how many tokens it used, with the rank each sort should give it.
type rankedSession struct {
	id    string
	calls int
	last  int
	cost  int64
	token int64
	rows  []usage.Row
}

// rankedSessions makes 30 sessions of one to four calls. Three sessions share a last call, some cost nothing
// that is measured, every fifth has no price at all, and the counts and tokens repeat, so every sort has ties.
func rankedSessions() []rankedSession {
	var out []rankedSession
	for i := 0; i < 30; i++ {
		rs := rankedSession{id: fmt.Sprintf("s-%02d", i), calls: 1 + i%4, last: 1000 + (i/3)*10}
		for j := 0; j < rs.calls; j++ {
			r := callRow("t-"+rs.id, j, rs.id, rs.last-(rs.calls-1-j))
			r.InputTokens, r.OutputTokens = p64(int64(i%5*10)), p64(1)
			r.USD = pf(float64(i%3) * 0.25)
			if i%5 == 0 {
				r.USD = nil
			}
			rs.rows = append(rs.rows, r)
			rs.token += int64(i%5*10) + 1
		}
		rs.cost = -1
		if i%5 != 0 {
			rs.cost = int64(rs.calls) * int64(i%3) * 250_000_000
		}
		out = append(out, rs)
	}
	return out
}

func (rs rankedSession) rank(sortName string) int64 {
	switch sortName {
	case "calls":
		return int64(rs.calls)
	case "tokens":
		return rs.token
	case "cost":
		return rs.cost
	}
	return int64(rs.last)
}

func expectedOrder(all []rankedSession, sortName string) []string {
	sorted := append([]rankedSession(nil), all...)
	sort.Slice(sorted, func(i, j int) bool {
		if a, b := sorted[i].rank(sortName), sorted[j].rank(sortName); a != b {
			return a > b
		}
		return sorted[i].id < sorted[j].id
	})
	ids := make([]string, len(sorted))
	for i, s := range sorted {
		ids[i] = s.id
	}
	return ids
}

func loadRanked(t *testing.T, s usage.Store) []rankedSession {
	all := rankedSessions()
	for _, rs := range all {
		add(t, s, rs.rows...)
	}
	return all
}

// walk lists every page of a sort, calling between pages with the ids just listed.
func walk(t *testing.T, s usage.Store, sortName string, size int, between func(page int, ids []string)) []string {
	t.Helper()
	var out []string
	page := usage.Page{Limit: size, Sort: sortName}
	for n := 0; ; n++ {
		got, next, err := s.Sessions(ctx, usage.Filter{}, page)
		if err != nil {
			t.Fatal(err)
		}
		ids := sessionIDs(got)
		out = append(out, ids...)
		if between != nil {
			between(n, ids)
		}
		if next == "" {
			return out
		}
		page.After = next
	}
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var sortCases = []contractCase{
	{"every sort ranks sessions from the greatest, ties by id, and pages without a gap", func(t *testing.T, s usage.Store) {
		all := loadRanked(t, s)
		for _, name := range []string{"", "last", "cost", "calls", "tokens"} {
			want := expectedOrder(all, name)
			if got := walk(t, s, name, 4, nil); !equalIDs(got, want) {
				t.Errorf("sort %q paged as\n%v\nwant\n%v", name, got, want)
			}
			whole, _, err := s.Sessions(ctx, usage.Filter{}, usage.Page{Limit: 100, Sort: name})
			if err != nil || !equalIDs(sessionIDs(whole), want) {
				t.Errorf("sort %q in one page = %v %v", name, sessionIDs(whole), err)
			}
		}
	}},
	{"a session with no measured call ranks last by cost, below one that cost nothing", func(t *testing.T, s usage.Store) {
		free, unpriced := callRow("a", 0, "free", 5), callRow("b", 0, "unpriced", 9)
		free.USD, unpriced.USD = pf(0), nil
		add(t, s, unpriced, free)
		got := must[[]usage.Session](t)(sessionsSorted(s, "cost"))
		if ids := sessionIDs(got); !equalIDs(ids, []string{"free", "unpriced"}) {
			t.Fatalf("order = %v", ids)
		}
	}},
	{"paging holds for every sort while the sessions already listed are used again", func(t *testing.T, s usage.Store) {
		all := loadRanked(t, s)
		for _, name := range []string{"last", "cost", "calls", "tokens"} {
			seen := map[string]int{}
			round := 0
			ids := walk(t, s, name, 4, func(page int, listed []string) {
				var more []usage.Row
				for _, id := range listed {
					r := callRow(fmt.Sprintf("more-%s-%d-%s", name, page, id), 0, id, 50_000+round)
					r.USD, r.InputTokens = pf(1), p64(1000)
					more = append(more, r)
					round++
				}
				add(t, s, more...)
			})
			for _, id := range ids {
				seen[id]++
			}
			for _, rs := range all {
				if seen[rs.id] != 1 {
					t.Fatalf("sort %s listed %s %d times", name, rs.id, seen[rs.id])
				}
			}
			if len(ids) != len(all) {
				t.Fatalf("sort %s listed %d sessions, want %d", name, len(ids), len(all))
			}
		}
	}},
	{"an unknown sort and a cursor from another listing are refused", func(t *testing.T, s usage.Store) {
		loadRanked(t, s)
		if _, _, err := s.Sessions(ctx, usage.Filter{}, usage.Page{Sort: "newest"}); !errors.Is(err, usage.ErrBadSort) {
			t.Fatalf("err = %v", err)
		}
		_, byCalls, err := s.Sessions(ctx, usage.Filter{}, usage.Page{Limit: 3, Sort: "calls"})
		if err != nil || byCalls == "" {
			t.Fatalf("cursor = %q %v", byCalls, err)
		}
		if _, _, err := s.Sessions(ctx, usage.Filter{}, usage.Page{Sort: "cost", After: byCalls}); !errors.Is(err, usage.ErrBadCursor) {
			t.Fatalf("a calls cursor in a cost listing: %v", err)
		}
		if _, _, err := s.Calls(ctx, "s-01", usage.Filter{}, usage.Page{After: byCalls}); !errors.Is(err, usage.ErrBadCursor) {
			t.Fatalf("a sessions cursor in Calls: %v", err)
		}
		_, callsNext, err := s.Calls(ctx, "s-03", usage.Filter{}, usage.Page{Limit: 1})
		if err != nil || callsNext == "" {
			t.Fatalf("calls cursor = %q %v", callsNext, err)
		}
		if _, _, err := s.Sessions(ctx, usage.Filter{}, usage.Page{After: callsNext}); !errors.Is(err, usage.ErrBadCursor) {
			t.Fatalf("a calls cursor in Sessions: %v", err)
		}
	}},
	{"a page holds at most a thousand", func(t *testing.T, s usage.Store) {
		rows := make([]usage.Row, 1200)
		for i := range rows {
			rows[i] = callRow("t", i, "one", i)
		}
		other := make([]usage.Row, 1100)
		for i := range other {
			other[i] = callRow(fmt.Sprintf("u%d", i), 0, fmt.Sprintf("s-%04d", i), i)
		}
		add(t, s, append(rows, other...)...)
		calls, next, err := s.Calls(ctx, "one", usage.Filter{}, usage.Page{Limit: 5000})
		if err != nil || len(calls) != 1000 || next == "" {
			t.Fatalf("calls: %d %q %v", len(calls), next, err)
		}
		sessions, next, err := s.Sessions(ctx, usage.Filter{}, usage.Page{Limit: 5000})
		if err != nil || len(sessions) != 1000 || next == "" {
			t.Fatalf("sessions: %d %q %v", len(sessions), next, err)
		}
	}},
}

func sessionsSorted(s usage.Store, name string) ([]usage.Session, error) {
	list, _, err := s.Sessions(ctx, usage.Filter{}, usage.Page{Limit: 100, Sort: name})
	return list, err
}
