package usage_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sort"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

type contractCase struct {
	name string
	run  func(t *testing.T, s usage.Store)
}

// TestContract holds every store to the same rules: the in-memory fake, SQLite, and Postgres when
// GUSTO_TEST_POSTGRES_URL names a server.
func TestContract(t *testing.T) {
	cases := append(append(append(append(append(append(storageCases, filterCases...), listingCases...), sortCases...), exportCases...), concurrencyCases...), groupCases...)
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) { c.run(t, b.open(t)) })
			}
		})
	}
}

var ctx = context.Background()

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func add(t *testing.T, s usage.Store, rows ...usage.Row) {
	t.Helper()
	if err := s.Add(ctx, rows); err != nil {
		t.Fatal(err)
	}
}

func totals(t *testing.T, s usage.Store, f usage.Filter) usage.Totals {
	t.Helper()
	return must[usage.Totals](t)(s.Totals(ctx, f))
}

func sameRow(a, b usage.Row) bool {
	if !a.At.Equal(b.At) {
		return false
	}
	a.At = b.At
	return reflect.DeepEqual(a, b)
}

var storageCases = []contractCase{
	{"add is idempotent and keeps the first row", func(t *testing.T, s usage.Store) {
		first := callRow("t1", 0, "s1", 1)
		again := first
		again.Model = "other"
		add(t, s, first, callRow("t1", 1, "s1", 2))
		add(t, s, again, callRow("t1", 1, "s1", 2), callRow("t1", 2, "s1", 3))
		if got := totals(t, s, usage.Filter{}).Calls; got != 3 {
			t.Fatalf("calls = %d, want 3", got)
		}
		rows, _, err := s.Calls(ctx, "s1", usage.Filter{}, usage.Page{})
		if err != nil || rows[2].Model != "sonnet" {
			t.Fatalf("first row not kept: %v %v", rows, err)
		}
	}},
	{"add takes more rows than one transaction holds", func(t *testing.T, s usage.Store) {
		var rows []usage.Row
		for i := 0; i < 2503; i++ {
			rows = append(rows, callRow("big", i, "s-big", i))
		}
		add(t, s, rows...)
		add(t, s, rows[1000:1500]...)
		if got := totals(t, s, usage.Filter{}).Calls; got != 2503 {
			t.Fatalf("calls = %d, want 2503", got)
		}
	}},
	{"a row comes back as it went in, and a missing count stays missing", func(t *testing.T, s usage.Store) {
		full := usage.Row{Task: "t", Line: 3, At: at(5), Session: "s", Person: "ada", Team: "core", Project: "web",
			Engine: "codex", Agent: "reviewer", Provider: "openai", Credential: "key-1", Model: "gpt",
			InputTokens: p64(0), OutputTokens: p64(7), CachedTokens: p64(2), USD: pf(0), Status: 429, TookMS: 31,
			Refused: "daily cap", Translated: "responses"}
		bare := usage.Row{Task: "t", Line: 4, At: at(4), Session: "s", Person: "ada", Engine: "codex", Provider: "openai", Model: "gpt"}
		add(t, s, full, bare)
		rows := must[[]usage.Row](t)(callsOf(s, "s", 50))
		if len(rows) != 2 || !sameRow(rows[0], full) || !sameRow(rows[1], bare) {
			t.Fatalf("rows = %+v", rows)
		}
		if bare, zero := rows[1], rows[0]; bare.InputTokens != nil || bare.USD != nil || zero.USD == nil || *zero.InputTokens != 0 {
			t.Fatal("nil and zero were confused")
		}
	}},
	{"totals count unmeasured and refused calls apart from the money", func(t *testing.T, s usage.Store) {
		measured := callRow("t", 0, "s", 1)
		measured.USD, measured.CachedTokens = pf(1.5), p64(4)
		free := callRow("t", 1, "s", 2)
		free.USD = pf(0)
		unpriced := callRow("t", 2, "s", 3)
		unpriced.USD, unpriced.InputTokens, unpriced.OutputTokens = nil, nil, nil
		refused := callRow("t", 3, "s", 4)
		refused.Refused, refused.USD = "cap", nil
		add(t, s, measured, free, unpriced, refused)
		got := totals(t, s, usage.Filter{})
		want := usage.Totals{Calls: 4, Unmeasured: 2, Refused: 1, InputTokens: 30, OutputTokens: 15, CachedTokens: 4, USD: 1.5}
		if got.USD = math.Round(got.USD*1e6) / 1e6; got != want {
			t.Fatalf("totals = %+v, want %+v", got, want)
		}
	}},
	{"an empty index answers with nothing", func(t *testing.T, s usage.Store) {
		list, next, err := s.Sessions(ctx, usage.Filter{}, usage.Page{})
		if err != nil || len(list) != 0 || next != "" {
			t.Fatalf("sessions = %v %q %v", list, next, err)
		}
		if got := totals(t, s, usage.Filter{}); got != (usage.Totals{}) {
			t.Fatalf("totals = %+v", got)
		}
		if got := must[usage.Facets](t)(s.Facets(ctx, usage.Filter{})); !reflect.DeepEqual(got, usage.Facets{}) {
			t.Fatalf("facets = %+v", got)
		}
		if got := must[map[string]int](t)(s.Indexed(ctx)); len(got) != 0 {
			t.Fatalf("indexed = %v", got)
		}
	}},
	{"indexed is one more than the highest line of each task", func(t *testing.T, s usage.Store) {
		add(t, s, callRow("a", 0, "s", 1), callRow("a", 1, "s", 2), callRow("a", 4, "s", 3), callRow("b", 0, "s", 4))
		if got := must[map[string]int](t)(s.Indexed(ctx)); !reflect.DeepEqual(got, map[string]int{"a": 5, "b": 1}) {
			t.Fatalf("indexed = %v", got)
		}
		add(t, s, callRow("a", 9, "s", 5))
		if got := must[map[string]int](t)(s.Indexed(ctx)); got["a"] != 10 {
			t.Fatalf("indexed = %v", got)
		}
	}},
	{"facets are the distinct values, sorted, that the filter leaves", func(t *testing.T, s usage.Store) {
		add(t, s, filterRows()...)
		got := must[usage.Facets](t)(s.Facets(ctx, usage.Filter{}))
		want := usage.Facets{People: []string{"ada", "bob", "cy_d"}, Teams: []string{"core", "ml"}, Projects: []string{"api", "web"},
			Engines: []string{"claude-code", "codex", "pi"}, Agents: []string{"reviewer"},
			Providers: []string{"anthropic", "bedrock", "openai"}, Models: []string{"Claude-3%", "gpt", "sonnet"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("facets = %+v", got)
		}
		got = must[usage.Facets](t)(s.Facets(ctx, usage.Filter{Person: "bob"}))
		if !reflect.DeepEqual(got.Models, []string{"gpt"}) || !reflect.DeepEqual(got.People, []string{"bob"}) {
			t.Fatalf("filtered facets = %+v", got)
		}
	}},
	{"a canceled context stops every call", func(t *testing.T, s usage.Store) {
		add(t, s, callRow("t", 0, "s", 1))
		done, cancel := context.WithCancel(ctx)
		cancel()
		_, _, sessionsErr := s.Sessions(done, usage.Filter{}, usage.Page{})
		_, _, callsErr := s.Calls(done, "s", usage.Filter{}, usage.Page{})
		_, totalsErr := s.Totals(done, usage.Filter{})
		_, facetsErr := s.Facets(done, usage.Filter{})
		_, indexedErr := s.Indexed(done)
		addErr := s.Add(done, []usage.Row{callRow("t", 1, "s", 2)})
		for name, err := range map[string]error{"sessions": sessionsErr, "calls": callsErr, "totals": totalsErr,
			"facets": facetsErr, "indexed": indexedErr, "add": addErr} {
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s: err = %v", name, err)
			}
		}
		if got := totals(t, s, usage.Filter{}).Calls; got != 1 {
			t.Fatalf("a canceled add held %d calls", got)
		}
	}},
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
