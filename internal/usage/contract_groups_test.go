package usage_test

import (
	"errors"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

func groupRows() []usage.Row {
	rows := filterRows()
	nobody := usage.Row{Task: "td", Line: 0, At: at(40), Session: "s-delta", Engine: "codex", Agent: "writer", Provider: "openai", Model: "gpt"}
	again := rows[0]
	again.Task, again.Agent, again.USD = "te", "planner", pf(2)
	return append(rows, nobody, again)
}

var groupCases = []contractCase{
	{"groups add up by person, the most spent first, with an empty person kept", func(t *testing.T, s usage.Store) {
		add(t, s, groupRows()...)
		groups := must[[]usage.Group](t)(s.Groups(ctx, usage.Filter{}, usage.ByPerson))
		want := []struct {
			value         string
			calls, agents int64
			usd           float64
		}{{"ada", 3, 2, 4}, {"bob", 1, 0, 1}, {"cy_d", 1, 0, 1}, {"", 1, 1, 0}}
		if len(groups) != len(want) {
			t.Fatalf("groups = %+v", groups)
		}
		for i, w := range want {
			g := groups[i]
			if g.Value != w.value || g.Calls != w.calls || g.Agents != w.agents || g.USD != w.usd {
				t.Errorf("group %d = %+v, want %+v", i, g, w)
			}
		}
		if groups[3].Unmeasured != 1 {
			t.Errorf("a call with no price is not counted as one: %+v", groups[3])
		}
	}},
	{"groups by engine add up to the totals of the same filter", func(t *testing.T, s usage.Store) {
		add(t, s, groupRows()...)
		filter := usage.Filter{From: at(10)}
		var calls int64
		var usd float64
		for _, g := range must[[]usage.Group](t)(s.Groups(ctx, filter, usage.ByEngine)) {
			calls, usd = calls+g.Calls, usd+g.USD
		}
		whole := totals(t, s, filter)
		if calls != whole.Calls || usd != whole.USD {
			t.Fatalf("groups add up to %d calls and $%v, the totals are %+v", calls, usd, whole)
		}
	}},
	{"groups of nothing are none, and an unknown grouping is refused", func(t *testing.T, s usage.Store) {
		if groups := must[[]usage.Group](t)(s.Groups(ctx, usage.Filter{}, usage.ByEngine)); groups != nil {
			t.Fatalf("an empty index has groups: %+v", groups)
		}
		if _, err := s.Groups(ctx, usage.Filter{}, "model; DROP TABLE usage_calls"); !errors.Is(err, usage.ErrBadGroup) {
			t.Fatalf("an unknown grouping: %v", err)
		}
	}},
}
