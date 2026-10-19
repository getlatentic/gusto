package taskindex_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
)

// filterRows are five tasks that differ in every field a filter looks at. The newest is e.
func filterRows() []taskindex.Row {
	return []taskindex.Row{
		with(task("a", 1), func(r *taskindex.Row) { r.State, r.Team, r.Instance, r.Thread = "passed", "core", "ins-1", "1" }),
		with(task("b", 2), func(r *taskindex.Row) {
			r.State, r.Asked, r.Repo, r.AgentType, r.Instance, r.Thread = "failed", "Bob@Acme.com", "acme/api", "claude-code", "ins-1", "2"
		}),
		with(task("c", 3), func(r *taskindex.Row) { r.State, r.Team, r.AgentType = "interrupted", "ml", "pi" }),
		with(task("d", 4), func(r *taskindex.Row) { r.State, r.Instance, r.Thread = "running", "ins-2", "1" }),
		task("e", 5),
	}
}

func filtered(t *testing.T, s taskindex.Store, f taskindex.Filter) []string {
	t.Helper()
	return list(t, s, f, taskindex.Page{})
}

var filterCases = []contractCase{
	{"the empty filter matches everything, the newest first", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		same(t, filtered(t, s, taskindex.Filter{}), []string{"e", "d", "c", "b", "a"})
	}},
	{"states match any of those named", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		same(t, filtered(t, s, taskindex.Filter{States: []string{"failed"}}), []string{"b"})
		same(t, filtered(t, s, taskindex.Filter{States: []string{"failed", "interrupted", "running"}}), []string{"d", "c", "b"})
		same(t, filtered(t, s, taskindex.Filter{States: []string{"nonsense"}}), []string{})
	}},
	{"the asker is matched without regard to case", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		same(t, filtered(t, s, taskindex.Filter{Asked: "bob@acme.com"}), []string{"b"})
		same(t, filtered(t, s, taskindex.Filter{Asked: "BOB@ACME.COM"}), []string{"b"})
		same(t, filtered(t, s, taskindex.Filter{Asked: "ADA@acme.com"}), []string{"e", "d", "c", "a"})
		same(t, filtered(t, s, taskindex.Filter{Asked: "bob"}), []string{})
	}},
	{"team, repository, agent type, instance and thread are exact", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		same(t, filtered(t, s, taskindex.Filter{Team: "core"}), []string{"a"})
		same(t, filtered(t, s, taskindex.Filter{Team: "COR"}), []string{})
		same(t, filtered(t, s, taskindex.Filter{Repo: "acme/api"}), []string{"b"})
		same(t, filtered(t, s, taskindex.Filter{AgentType: "pi"}), []string{"c"})
		same(t, filtered(t, s, taskindex.Filter{Instance: "ins-1"}), []string{"b", "a"})
		same(t, filtered(t, s, taskindex.Filter{Instance: "ins-1", Thread: "2"}), []string{"b"})
		same(t, filtered(t, s, taskindex.Filter{Thread: "1"}), []string{"d", "a"})
	}},
	{"the time of asking starts included and ends excluded", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		at := func(second int) time.Time { return epoch.Add(time.Duration(second) * time.Second) }
		same(t, filtered(t, s, taskindex.Filter{QueuedFrom: at(3)}), []string{"e", "d", "c"})
		same(t, filtered(t, s, taskindex.Filter{QueuedTo: at(3)}), []string{"b", "a"})
		same(t, filtered(t, s, taskindex.Filter{QueuedFrom: at(2), QueuedTo: at(4)}), []string{"c", "b"})
		same(t, filtered(t, s, taskindex.Filter{QueuedFrom: at(9)}), []string{})
	}},
	{"filters narrow one another", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		same(t, filtered(t, s, taskindex.Filter{Asked: "ada@acme.com", States: []string{"passed", "queued"}, Instance: ""}), []string{"e", "a"})
		same(t, filtered(t, s, taskindex.Filter{Asked: "ada@acme.com", Team: "ml", AgentType: "pi"}), []string{"c"})
		same(t, filtered(t, s, taskindex.Filter{Asked: "bob@acme.com", Team: "ml"}), []string{})
	}},
	{"counts are by state under the filter", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		got, err := s.Count(ctx, taskindex.Filter{})
		want := map[string]int{"passed": 1, "failed": 1, "interrupted": 1, "running": 1, "queued": 1}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v %v", got, err)
		}
		got, _ = s.Count(ctx, taskindex.Filter{Asked: "ada@acme.com", States: []string{"queued", "running"}})
		if !reflect.DeepEqual(got, map[string]int{"running": 1, "queued": 1}) {
			t.Fatalf("got %v", got)
		}
	}},
	{"agent types are the distinct ones the filter leaves, in order", func(t *testing.T, s taskindex.Store) {
		put(t, s, filterRows()...)
		got, err := s.AgentTypes(ctx, taskindex.Filter{})
		if err != nil || !reflect.DeepEqual(got, []string{"claude-code", "codex", "pi"}) {
			t.Fatalf("got %v %v", got, err)
		}
		got, _ = s.AgentTypes(ctx, taskindex.Filter{Asked: "bob@acme.com"})
		if !reflect.DeepEqual(got, []string{"claude-code"}) {
			t.Fatalf("got %v", got)
		}
	}},
}

var searchCases = []contractCase{
	{"search looks in the id, brief, asker and repository without regard to case", func(t *testing.T, s taskindex.Store) {
		put(t, s,
			with(task("20261001-fix-login", 1), func(r *taskindex.Row) { r.Brief, r.Repo, r.Asked = "Repair the form", "acme/web", "ada@acme.com" }),
			with(task("20261002-docs", 2), func(r *taskindex.Row) { r.Brief, r.Repo, r.Asked = "Write DOCS", "acme/site", "bob@corp.io" }),
			with(task("20261003-other", 3), func(r *taskindex.Row) { r.Brief, r.Repo, r.Asked = "Tidy", "acme/api", "cy@acme.com" }))
		for search, want := range map[string][]string{
			"LOGIN": {"20261001-fix-login"}, "repair": {"20261001-fix-login"}, "docs": {"20261002-docs"},
			"BOB@CORP": {"20261002-docs"}, "acme/site": {"20261002-docs"}, "acme": {"20261003-other", "20261002-docs", "20261001-fix-login"},
			"20261003": {"20261003-other"}, "nothing like it": {},
		} {
			same(t, filtered(t, s, taskindex.Filter{Search: search}), want)
		}
	}},
	{"search takes percent and underscore as themselves", func(t *testing.T, s taskindex.Store) {
		put(t, s,
			with(task("p", 1), func(r *taskindex.Row) { r.Brief = "grow 50% faster" }),
			with(task("q", 2), func(r *taskindex.Row) { r.Brief = "grow 50 faster" }),
			with(task("u", 3), func(r *taskindex.Row) { r.Brief = "snake_case name" }),
			with(task("v", 4), func(r *taskindex.Row) { r.Brief = "snakeXcase name" }),
			with(task("w", 5), func(r *taskindex.Row) { r.Brief = `back\slash` }))
		same(t, filtered(t, s, taskindex.Filter{Search: "50%"}), []string{"p"})
		same(t, filtered(t, s, taskindex.Filter{Search: "%"}), []string{"p"})
		same(t, filtered(t, s, taskindex.Filter{Search: "e_c"}), []string{"u"})
		same(t, filtered(t, s, taskindex.Filter{Search: "_"}), []string{"u"})
		same(t, filtered(t, s, taskindex.Filter{Search: `\`}), []string{"w"})
	}},
	{"search folds the case of letters beyond ASCII alike in every store", func(t *testing.T, s taskindex.Store) {
		put(t, s, with(task("a", 1), func(r *taskindex.Row) { r.Brief = "Über ÉCOLE" }))
		same(t, filtered(t, s, taskindex.Filter{Search: "über"}), []string{"a"})
		same(t, filtered(t, s, taskindex.Filter{Search: "école"}), []string{"a"})
	}},
	{"search does not match across the fields", func(t *testing.T, s taskindex.Store) {
		put(t, s, with(task("ab", 1), func(r *taskindex.Row) { r.Brief, r.Asked, r.Repo = "cd", "ef", "gh" }))
		same(t, filtered(t, s, taskindex.Filter{Search: "abcd"}), []string{})
		same(t, filtered(t, s, taskindex.Filter{Search: "ab\x1fcd"}), []string{})
		same(t, filtered(t, s, taskindex.Filter{Search: "cd"}), []string{"ab"})
	}},
	{"search does not look in the other fields", func(t *testing.T, s taskindex.Store) {
		put(t, s, with(task("a", 1), func(r *taskindex.Row) { r.Team, r.Model, r.StoppedBy = "zeta", "omega", "kappa" }))
		for _, word := range []string{"zeta", "omega", "kappa", "codex"} {
			same(t, filtered(t, s, taskindex.Filter{Search: word}), []string{})
		}
	}},
}
