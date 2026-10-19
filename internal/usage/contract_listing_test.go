package usage_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

var listingCases = []contractCase{
	{"a session is its calls added up, with what it used in order of first use", func(t *testing.T, s usage.Store) {
		mk := func(line, sec int, provider, model, task string) usage.Row {
			r := callRow(task, line, "s", sec)
			r.Provider, r.Model = provider, model
			return r
		}
		first := mk(0, 10, "b", "m2", "t-b")
		first.Person, first.Agent, first.Engine = "ada", "reviewer", "codex"
		later := mk(0, 40, "c", "m3", "t-c")
		later.Person, later.Agent, later.Engine = "bob", "other", "pi"
		add(t, s, later, mk(1, 20, "a", "m1", "t-a"), first, mk(2, 30, "b", "m2", "t-a"))
		got := must[[]usage.Session](t)(sessionsOf(s, usage.Filter{}))
		if len(got) != 1 {
			t.Fatalf("sessions = %+v", got)
		}
		g := got[0]
		if g.ID != "s" || g.Person != "ada" || g.Agent != "reviewer" || g.Engine != "codex" || !g.First.Equal(at(10)) || !g.Last.Equal(at(40)) {
			t.Errorf("session = %+v", g)
		}
		if !reflect.DeepEqual(g.Providers, []string{"b", "a", "c"}) || !reflect.DeepEqual(g.Models, []string{"m2", "m1", "m3"}) ||
			!reflect.DeepEqual(g.Tasks, []string{"t-b", "t-a", "t-c"}) {
			t.Errorf("used = %v %v %v", g.Providers, g.Models, g.Tasks)
		}
		if g.Calls != 4 || g.InputTokens != 40 || g.OutputTokens != 20 {
			t.Errorf("totals = %+v", g.Totals)
		}
	}},
	{"a session's totals count only the calls that match", func(t *testing.T, s usage.Store) {
		a, b := callRow("t", 0, "s", 10), callRow("t", 1, "s", 50)
		b.Model, b.USD = "opus", nil
		add(t, s, a, b)
		got := must[[]usage.Session](t)(sessionsOf(s, usage.Filter{Model: "opus"}))
		if len(got) != 1 || got[0].Calls != 1 || got[0].Unmeasured != 1 || !got[0].First.Equal(at(50)) || !got[0].Last.Equal(at(50)) ||
			!reflect.DeepEqual(got[0].Models, []string{"opus"}) {
			t.Fatalf("sessions = %+v", got)
		}
	}},
	{"sessions list the most recently used first, ties by id", func(t *testing.T, s usage.Store) {
		add(t, s, callRow("t1", 0, "s-old", 1), callRow("t2", 0, "s-b", 20), callRow("t3", 0, "s-a", 20),
			callRow("t4", 0, "s-new", 30), callRow("t1", 1, "s-old", 25))
		got := sessionIDs(must[[]usage.Session](t)(sessionsOf(s, usage.Filter{})))
		if want := []string{"s-new", "s-old", "s-a", "s-b"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}},
	{"paging reaches every session once, even while sessions are used", walkWhileAdding},
	{"calls list the latest first and page without a gap", func(t *testing.T, s usage.Store) {
		var rows []usage.Row
		for i := 0; i < 25; i++ {
			rows = append(rows, callRow(fmt.Sprintf("t%d", i%3), i, "s", i/4))
		}
		add(t, s, rows...)
		var keys []string
		page := usage.Page{Limit: 4}
		for {
			got, next, err := s.Calls(ctx, "s", usage.Filter{}, page)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range got {
				keys = append(keys, fmt.Sprintf("%s/%d", r.Task, r.Line))
			}
			if next == "" {
				break
			}
			page.After = next
		}
		all := must[[]usage.Row](t)(callsOf(s, "s", 100))
		var want []string
		for _, r := range all {
			want = append(want, fmt.Sprintf("%s/%d", r.Task, r.Line))
		}
		if len(keys) != 25 || !reflect.DeepEqual(keys, want) {
			t.Fatalf("paged %v, whole %v", keys, want)
		}
		for i := 1; i < len(all); i++ {
			if all[i].At.After(all[i-1].At) {
				t.Fatalf("calls are not latest first at %d", i)
			}
		}
	}},
	{"an unknown session is not found", func(t *testing.T, s usage.Store) {
		add(t, s, callRow("t", 0, "s", 1))
		if _, _, err := s.Calls(ctx, "nope", usage.Filter{}, usage.Page{}); !errors.Is(err, usage.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	}},
	{"a cursor that no listing made is refused", func(t *testing.T, s usage.Store) {
		add(t, s, callRow("t", 0, "s", 1))
		if _, _, err := s.Sessions(ctx, usage.Filter{}, usage.Page{After: "%%%"}); !errors.Is(err, usage.ErrBadCursor) {
			t.Fatalf("sessions err = %v", err)
		}
		if _, _, err := s.Calls(ctx, "s", usage.Filter{}, usage.Page{After: "%%%"}); !errors.Is(err, usage.ErrBadCursor) {
			t.Fatalf("calls err = %v", err)
		}
	}},
}

func callsOf(s usage.Store, session string, limit int) ([]usage.Row, error) {
	rows, _, err := s.Calls(ctx, session, usage.Filter{}, usage.Page{Limit: limit})
	return rows, err
}

// walkWhileAdding pages through 120 sessions, three to a moment, while calls land on the sessions already
// listed and brand new sessions begin. No session may come twice and none that was there may be missed.
func walkWhileAdding(t *testing.T, s usage.Store) {
	var rows []usage.Row
	ids := numbered("s-", 120)
	for i, id := range ids {
		rows = append(rows, callRow("t-"+id, 0, id, 100+i/3))
	}
	add(t, s, rows...)
	seen := map[string]int{}
	page := usage.Page{Limit: 7}
	for n := 0; ; n++ {
		got, next, err := s.Sessions(ctx, usage.Filter{}, page)
		if err != nil {
			t.Fatal(err)
		}
		for _, sess := range got {
			seen[sess.ID]++
		}
		var more []usage.Row
		for k, sess := range got {
			more = append(more, callRow(fmt.Sprintf("late-%d-%d", n, k), 0, sess.ID, 10_000+n))
		}
		more = append(more, callRow(fmt.Sprintf("fresh-%d", n), 0, fmt.Sprintf("fresh-%d", n), 20_000+n))
		add(t, s, more...)
		if next == "" {
			break
		}
		page.After = next
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("%s listed %d times", id, seen[id])
		}
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("%s listed %d times", id, n)
		}
	}
}
