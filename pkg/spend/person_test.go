package spend

import (
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

// A person is held to their month's budget across all their tasks, and nobody else is held to it.
func TestAPersonIsHeldToTheirBudgetAcrossTheirTasks(t *testing.T) {
	ledger := NewLedger(Policy{Prices: map[string]Price{"m": {InputPerMillion: 1}}})
	budget := 2.0
	dollar := 1.0
	call := Call{Path: "/v1/chat/completions", Model: "m", AtUnix: time.Now().Unix(), Status: 200, USD: &dollar}
	ada := func(id string) {
		terms := TermsOf(job.Spec{Harness: "pi", Person: "Ada@Acme.com"})
		terms.PersonUSD = &budget
		ledger.Begin(id, terms)
	}
	for _, task := range []string{"t1", "t2"} {
		ada(task)
		if refusal := ledger.Allow(task, "", "m", false); refusal != nil {
			t.Fatalf("%s was refused before the budget was spent: %+v", task, refusal)
		}
		ledger.Count(task, call)
	}
	ada("t3")
	refusal := ledger.Allow("t3", "", "m", false)
	if refusal == nil || refusal.Cap != "person" || refusal.SpentUSD != 2 || refusal.LimitUSD != 2 {
		t.Fatalf("a third task past the budget was let through: %+v", refusal)
	}
	if spent := ledger.SpentThisMonth(ByPerson, "ada@acme.com"); spent != 2 {
		t.Errorf("ada spent %v", spent)
	}
	ledger.Begin("t4", TermsOf(job.Spec{Harness: "pi", Person: "bob@acme.com"}))
	if refusal := ledger.Allow("t4", "", "m", false); refusal != nil {
		t.Errorf("bob was held to ada's budget: %+v", refusal)
	}
}
