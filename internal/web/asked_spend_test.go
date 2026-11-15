package web

import (
	"fmt"
	"net/url"
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

	ledger.UsePersonBudgets(func(person string) (float64, bool) {
		settings, err := here.server.orgSettings.Read()
		if err != nil {
			return 0, false
		}
		return settings.MemberBudget(person)
	})
	ledger.Begin("bob-again", spend.Terms{Person: "bob@acme.com"})
	refusal := ledger.Allow("bob-again", "platform", "m", false)
	if refusal == nil || refusal.Cap != "person" || refusal.SpentUSD != 5 {
		t.Fatalf("the proxy does not hold bob to his budget: %+v", refusal)
	}

	page := budgets(t, here)
	if bob := budgetRowOf(t, page, "bob@acme.com").Budget; bob.Spent != 5 || !bob.Passed {
		t.Errorf("Budgets does not show bob's $5.00 against his budget, reached: %+v", bob)
	}
	if ada := budgetRowOf(t, page, "ada@acme.com").Budget; ada.Spent == 5 || ada.Passed {
		t.Errorf("Budgets counts what bob asked of ada's agent against ada: %+v", ada)
	}

	page2 := people(t, here, "")
	if bob := personIn(t, page2, "bob@acme.com"); bob.spentOf() != 5 || !bob.Budget.Passed {
		t.Errorf("People does not show bob's $5.00, reached: %+v", bob)
	}
	if ada := personIn(t, page2, "ada@acme.com"); ada.spentOf() == 5 {
		t.Errorf("People counts bob's asks against ada: %+v", ada)
	}

	var split []string
	for _, line := range overview(t, here).Spent.People {
		split = append(split, fmt.Sprintf("%s %.2f %d", line.Label, line.USD, line.Calls))
	}
	if len(split) < 2 || !strings.HasPrefix(split[0], "Bob 5.00") || !strings.HasPrefix(split[1], "Ada 0.00 0") {
		t.Errorf("the overview does not split the month by who asked: %v", split)
	}
}
