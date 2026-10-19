package usage_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

func key(r usage.Row) string { return fmt.Sprintf("%s/%d", r.Task, r.Line) }

// exportRows makes 2,300 calls over 7 sessions, many at the same instant, so that a batch ends among ties.
func exportRows() []usage.Row {
	var rows []usage.Row
	for i := 0; i < 2300; i++ {
		r := callRow(fmt.Sprintf("task-%d", i%11), i, fmt.Sprintf("s-%d", i%7), i/5)
		if i%3 == 0 {
			r.Person = "bob"
		}
		rows = append(rows, r)
	}
	return rows
}

func exported(t *testing.T, s usage.Store, f usage.Filter) []usage.Row {
	t.Helper()
	var out []usage.Row
	if err := s.Export(ctx, f, func(r usage.Row) error { out = append(out, r); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

// pagedCalls reads the calls of every session that matches, page by page.
func pagedCalls(t *testing.T, s usage.Store, f usage.Filter) map[string]usage.Row {
	t.Helper()
	out := map[string]usage.Row{}
	for _, id := range walk(t, s, "", 3, nil) {
		page := usage.Page{Limit: 37}
		for {
			rows, next, err := s.Calls(ctx, id, f, page)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				out[key(r)] = r
			}
			if next == "" {
				break
			}
			page.After = next
		}
	}
	return out
}

var exportCases = []contractCase{
	{"export gives the calls latest first, the same as paging Calls over every session", func(t *testing.T, s usage.Store) {
		add(t, s, exportRows()...)
		for _, f := range []usage.Filter{{}, {Person: "bob"}, {From: at(100), To: at(300)}} {
			got := exported(t, s, f)
			paged := pagedCalls(t, s, f)
			if len(got) != len(paged) {
				t.Fatalf("%+v: export %d calls, paging %d", f, len(got), len(paged))
			}
			for i, r := range got {
				if p, ok := paged[key(r)]; !ok || !sameRow(p, r) {
					t.Fatalf("%+v: %s differs", f, key(r))
				}
				if i > 0 && !latestFirst(got[i-1], r) {
					t.Fatalf("%+v: %s should not follow %s", f, key(r), key(got[i-1]))
				}
			}
		}
		if n := len(exported(t, s, usage.Filter{})); n != 2300 {
			t.Fatalf("exported %d", n)
		}
	}},
	{"export stops at the first error from fn", func(t *testing.T, s usage.Store) {
		add(t, s, exportRows()...)
		stop, calls := errors.New("enough"), 0
		err := s.Export(ctx, usage.Filter{}, func(usage.Row) error {
			calls++
			if calls == 1500 {
				return stop
			}
			return nil
		})
		if !errors.Is(err, stop) || calls != 1500 {
			t.Fatalf("err = %v after %d calls", err, calls)
		}
	}},
	{"export honors the context", func(t *testing.T, s usage.Store) {
		add(t, s, exportRows()...)
		done, cancel := context.WithCancel(ctx)
		calls := 0
		err := s.Export(done, usage.Filter{}, func(usage.Row) error {
			if calls++; calls == 1200 {
				cancel()
			}
			return nil
		})
		if !errors.Is(err, context.Canceled) || calls > 1200 {
			t.Fatalf("err = %v after %d calls", err, calls)
		}
		cancel()
		if err := s.Export(done, usage.Filter{}, func(usage.Row) error { return nil }); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	}},
	{"calls narrowed by a filter", func(t *testing.T, s usage.Store) {
		a, b, c := callRow("t", 0, "s", 10), callRow("t", 1, "s", 20), callRow("t", 2, "s", 30)
		b.Model, c.Model, c.Person = "opus", "opus", "bob"
		add(t, s, a, b, c, callRow("u", 0, "other", 15))
		list := func(f usage.Filter) []string {
			rows, _, err := s.Calls(ctx, "s", f, usage.Page{})
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, r := range rows {
				out = append(out, key(r))
			}
			return out
		}
		for _, c := range []struct {
			f    usage.Filter
			want []string
		}{
			{usage.Filter{}, []string{"t/2", "t/1", "t/0"}},
			{usage.Filter{Model: "opus"}, []string{"t/2", "t/1"}},
			{usage.Filter{Model: "opus", Person: "ada"}, []string{"t/1"}},
			{usage.Filter{From: at(15), To: at(30)}, []string{"t/1"}},
			{usage.Filter{Search: "BOB"}, []string{"t/2"}},
		} {
			if got := list(c.f); !equalIDs(got, c.want) {
				t.Errorf("%+v: %v, want %v", c.f, got, c.want)
			}
		}
		rows, next, err := s.Calls(ctx, "s", usage.Filter{Provider: "nobody"}, usage.Page{})
		if err != nil || len(rows) != 0 || next != "" {
			t.Fatalf("no match: %v %q %v", rows, next, err)
		}
		if _, _, err := s.Calls(ctx, "nope", usage.Filter{Model: "opus"}, usage.Page{}); !errors.Is(err, usage.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
		page := usage.Page{Limit: 1}
		var paged []string
		for {
			rows, next, err := s.Calls(ctx, "s", usage.Filter{Model: "opus"}, page)
			if err != nil {
				t.Fatal(err)
			}
			paged = append(paged, key(rows[0]))
			if next == "" {
				break
			}
			page.After = next
		}
		if !equalIDs(paged, []string{"t/2", "t/1"}) {
			t.Fatalf("paged %v", paged)
		}
	}},
	{"reset removes every row and says how many", func(t *testing.T, s usage.Store) {
		if n, err := s.Reset(ctx); err != nil || n != 0 {
			t.Fatalf("reset of nothing = %d %v", n, err)
		}
		add(t, s, callRow("t", 0, "s", 1), callRow("t", 1, "s", 2), callRow("u", 0, "v", 3))
		if n, err := s.Reset(ctx); err != nil || n != 3 {
			t.Fatalf("reset = %d %v", n, err)
		}
		if got := totals(t, s, usage.Filter{}).Calls; got != 0 {
			t.Fatalf("calls after reset = %d", got)
		}
		if got := must[map[string]int](t)(s.Indexed(ctx)); len(got) != 0 {
			t.Fatalf("indexed = %v", got)
		}
		add(t, s, callRow("t", 0, "s", 1))
		if got := totals(t, s, usage.Filter{}).Calls; got != 1 {
			t.Fatalf("calls after re-adding = %d", got)
		}
		done, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := s.Reset(done); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
		if got := totals(t, s, usage.Filter{}).Calls; got != 1 {
			t.Fatal("a canceled reset deleted rows")
		}
	}},
	{"facets leave out the people a person filter excludes", func(t *testing.T, s usage.Store) {
		add(t, s, filterRows()...)
		got := must[usage.Facets](t)(s.Facets(ctx, usage.Filter{Person: "ada"}))
		want := usage.Facets{People: []string{"ada"}, Teams: []string{"core"}, Projects: []string{"web"},
			Engines: []string{"claude-code"}, Agents: []string{"reviewer"}, Providers: []string{"anthropic"}, Models: []string{"sonnet"}}
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Fatalf("facets = %+v", got)
		}
		none := must[usage.Facets](t)(s.Facets(ctx, usage.Filter{Person: "nobody"}))
		if fmt.Sprintf("%+v", none) != fmt.Sprintf("%+v", usage.Facets{}) {
			t.Fatalf("facets of nobody = %+v", none)
		}
	}},
}

func latestFirst(a, b usage.Row) bool {
	if !a.At.Equal(b.At) {
		return a.At.After(b.At)
	}
	if a.Task != b.Task {
		return a.Task > b.Task
	}
	return a.Line > b.Line
}
