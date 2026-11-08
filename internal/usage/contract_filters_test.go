package usage_test

import (
	"reflect"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

func filterRows() []usage.Row {
	a0 := usage.Row{Task: "ta", Line: 0, At: at(0), Session: "s-alpha", Person: "ada", Team: "core", Project: "web",
		Engine: "claude-code", Agent: "reviewer", Provider: "anthropic", Model: "sonnet", USD: pf(1)}
	a1 := a0
	a1.Line, a1.At = 1, at(10)
	b := usage.Row{Task: "tb", Line: 0, At: at(20), Session: "s-beta", Person: "bob", Team: "ml", Project: "api",
		Engine: "codex", Provider: "openai", Credential: "key-1", Model: "gpt", USD: pf(1)}
	c := usage.Row{Task: "tc", Line: 0, At: at(30), Session: "s-gamma", Person: "cy_d", Engine: "pi",
		Provider: "bedrock", Model: "Claude-3%", USD: pf(1)}
	return []usage.Row{a0, a1, b, c}
}

type filterCase struct {
	name     string
	filter   usage.Filter
	calls    int64
	sessions []string
}

var filterTable = []filterCase{
	{"empty filter matches everything", usage.Filter{}, 4, []string{"s-alpha", "s-beta", "s-gamma"}},
	{"person", usage.Filter{Person: "ada"}, 2, []string{"s-alpha"}},
	{"team", usage.Filter{Team: "ml"}, 1, []string{"s-beta"}},
	{"project", usage.Filter{Project: "web"}, 2, []string{"s-alpha"}},
	{"engine", usage.Filter{Engine: "pi"}, 1, []string{"s-gamma"}},
	{"agent", usage.Filter{Agent: "reviewer"}, 2, []string{"s-alpha"}},
	{"provider", usage.Filter{Provider: "bedrock"}, 1, []string{"s-gamma"}},
	{"model", usage.Filter{Model: "gpt"}, 1, []string{"s-beta"}},
	{"fields are anded", usage.Filter{Person: "ada", Model: "gpt"}, 0, nil},
	{"a field is matched whole and with case", usage.Filter{Person: "AD"}, 0, nil},
	{"from is inclusive", usage.Filter{From: at(10)}, 3, []string{"s-alpha", "s-beta", "s-gamma"}},
	{"from after a call leaves it out", usage.Filter{From: at(11)}, 2, []string{"s-beta", "s-gamma"}},
	{"to is exclusive", usage.Filter{To: at(20)}, 2, []string{"s-alpha"}},
	{"from and to make a window", usage.Filter{From: at(10), To: at(30)}, 2, []string{"s-alpha", "s-beta"}},
	{"an empty window", usage.Filter{From: at(5), To: at(5)}, 0, nil},
	{"search ignores case", usage.Filter{Search: "ALPHA"}, 2, []string{"s-alpha"}},
	{"search reaches the engine and the model", usage.Filter{Search: "claude"}, 3, []string{"s-alpha", "s-gamma"}},
	{"search reaches the person", usage.Filter{Search: "BoB"}, 1, []string{"s-beta"}},
	{"search reaches the agent", usage.Filter{Search: "review"}, 2, []string{"s-alpha"}},
	{"search reaches the provider", usage.Filter{Search: "OpenAI"}, 1, []string{"s-beta"}},
	{"search reaches the model", usage.Filter{Search: "gp"}, 1, []string{"s-beta"}},
	{"search does not reach the team, project or key", usage.Filter{Search: "core"}, 0, nil},
	{"search does not reach the credential", usage.Filter{Search: "key-1"}, 0, nil},
	{"search does not join two fields", usage.Filter{Search: "adaclaude"}, 0, nil},
	{"percent is literal", usage.Filter{Search: "%"}, 1, []string{"s-gamma"}},
	{"percent in the middle is literal", usage.Filter{Search: "e-3%"}, 1, []string{"s-gamma"}},
	{"underscore is literal", usage.Filter{Search: "_"}, 1, []string{"s-gamma"}},
	{"underscore does not stand for a letter", usage.Filter{Search: "c_d"}, 0, nil},
	{"backslash is literal", usage.Filter{Search: `\`}, 0, nil},
	{"search with a filter", usage.Filter{Search: "claude", Person: "ada"}, 2, []string{"s-alpha"}},
	{"only the sessions listed", usage.Filter{OnlySessions: true, Sessions: []string{"s-alpha", "s-gamma", "s-none"}}, 3, []string{"s-alpha", "s-gamma"}},
	{"only no sessions is nothing", usage.Filter{OnlySessions: true}, 0, nil},
	{"sessions without only are ignored", usage.Filter{Sessions: []string{"s-beta"}}, 4, []string{"s-alpha", "s-beta", "s-gamma"}},
	{"sessions are anded with a field", usage.Filter{OnlySessions: true, Sessions: []string{"s-alpha", "s-beta"}, Person: "bob"}, 1, []string{"s-beta"}},
}

var filterCases = []contractCase{
	{"filters pick the calls they should", func(t *testing.T, s usage.Store) {
		add(t, s, filterRows()...)
		for _, c := range filterTable {
			if got := totals(t, s, c.filter); got.Calls != c.calls {
				t.Errorf("%s: totals counted %d calls, want %d", c.name, got.Calls, c.calls)
			}
			list := must[[]usage.Session](t)(sessionsOf(s, c.filter))
			var calls int64
			for _, sess := range list {
				calls += sess.Calls
			}
			if got := sortedCopy(sessionIDs(list)); !reflect.DeepEqual(got, c.sessions) && (len(got) > 0 || len(c.sessions) > 0) {
				t.Errorf("%s: sessions = %v, want %v", c.name, got, c.sessions)
			}
			if calls != c.calls {
				t.Errorf("%s: session totals add to %d calls, want %d", c.name, calls, c.calls)
			}
		}
	}},
}

func sessionsOf(s usage.Store, f usage.Filter) ([]usage.Session, error) {
	list, _, err := s.Sessions(ctx, f, usage.Page{Limit: 100})
	return list, err
}
