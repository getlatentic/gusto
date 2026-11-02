package web

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/pkg/spend"
)

// Bob asks Ada's agent for work. The usage index counts the calls against Ada, who owns the agent and pays for it, but the
// proxy holds Bob to his budget for them. Every figure shown beside a budget is the one the proxy holds a person to.
func TestSpendShownAgainstABudgetIsWhatThePersonAskedForAsTheProxyCountsIt(t *testing.T) {
	members := []roster.Member{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"}}
	here := withMonth(t, members, callNow("bob-on-adas-agent", "ada@acme.com", "Codex", "Ada's agent", 5))
	ledger := spend.NewLedger(spend.Policy{})
	here.server.UseLedger(ledger)
	here.ask(t, "POST", "/admin/budget", url.Values{"each": {"4"}})
	spendFor(ledger, "bob@acme.com", 5)

	limit := 4.0
	ledger.Begin("bob-again", spend.Terms{Person: "bob@acme.com", PersonUSD: &limit})
	refusal := ledger.Allow("bob-again", "platform", "m", false)
	if refusal == nil || refusal.Cap != "person" || refusal.SpentUSD != 5 {
		t.Fatalf("the proxy does not hold bob to his budget: %+v", refusal)
	}

	page := budgets(t, here)
	if bob := rowOf(t, page, "bob@acme.com"); !strings.Contains(bob, "$5.00") || !strings.Contains(bob, "reached") {
		t.Errorf("Budgets does not show bob's $5.00 against his budget, reached:\n%s", bob)
	}
	if ada := rowOf(t, page, "ada@acme.com"); strings.Contains(ada, "$5.00") || strings.Contains(ada, "reached") {
		t.Errorf("Budgets counts what bob asked of ada's agent against ada:\n%s", ada)
	}

	people := here.ask(t, "GET", "/admin/people", nil).Body.String()
	if bob := rowOf(t, people, "bob@acme.com"); !strings.Contains(bob, "$5.00") || !strings.Contains(bob, "reached") {
		t.Errorf("People does not show bob's $5.00, reached:\n%s", bob)
	}
	if ada := rowOf(t, people, "ada@acme.com"); strings.Contains(ada, "$5.00") {
		t.Errorf("People counts bob's asks against ada:\n%s", ada)
	}

	split := regexp.MustCompile(`(?s)By person, what they asked for</h2>.*?</section>`).FindString(overview(t, here))
	bob, ada := strings.Index(split, ">Bob</span>"), strings.Index(split, ">Ada</span>")
	if bob < 0 || ada < bob || !strings.Contains(split[bob:ada], "$5.00") || !strings.Contains(split[ada:], "nothing yet") {
		t.Errorf("the overview does not split the month by who asked:\n%s", split)
	}
}
