package spend

import (
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

func TestARoomsAgentIsHeldToItsAllowanceAcrossItsTurns(t *testing.T) {
	ledger := NewLedger(Policy{Prices: map[string]Price{"m": {InputPerMillion: 1}}})
	allowance := 2.0
	spec := job.Spec{Session: "room", Agent: "a1", Harness: "pi", Budget: job.Budget{MonthUSD: &allowance}}
	dollar := 1.0
	call := Call{Path: "/v1/chat/completions", Model: "m", AtUnix: time.Now().Unix(), Status: 200, USD: &dollar}
	for _, turn := range []string{"t1", "t2"} {
		ledger.Begin(turn, TermsOf(spec))
		if refusal := ledger.Allow(turn, "", "m"); refusal != nil {
			t.Fatalf("%s was refused before the allowance was spent: %+v", turn, refusal)
		}
		ledger.Count(turn, call)
	}
	ledger.Begin("t3", TermsOf(spec))
	refusal := ledger.Allow("t3", "", "m")
	if refusal == nil || refusal.Cap != "allowance" || refusal.SpentUSD != 2 {
		t.Fatalf("a third turn past the allowance was let through: %+v", refusal)
	}
	if spent := ledger.SpentThisMonth(ByAllowance, AllowanceOf("room", "a1")); spent != 2 {
		t.Errorf("spent %v", spent)
	}
	other := job.Spec{Session: "room", Agent: "a2", Harness: "pi"}
	ledger.Begin("t4", TermsOf(other))
	if refusal := ledger.Allow("t4", "", "m"); refusal != nil {
		t.Errorf("another agent of the room was held to the first's allowance: %+v", refusal)
	}
	if report := ledger.Report(monthOf(time.Now())); len(report.Allowances) != 1 || report.Allowances[0].Name != "room/a1" {
		t.Errorf("the report's allowances are %+v", report.Allowances)
	}
}

func TestATaskThatIsNotARoomsTurnHasNoAllowance(t *testing.T) {
	ledger := NewLedger(Policy{Prices: map[string]Price{"m": {InputPerMillion: 1}}})
	ledger.Begin("t", TermsOf(job.Spec{Harness: "pi"}))
	dollar := 1.0
	ledger.Count("t", Call{Path: "/v1/chat/completions", Model: "m", AtUnix: time.Now().Unix(), Status: 200, USD: &dollar})
	if report := ledger.Report(monthOf(time.Now())); len(report.Allowances) != 0 {
		t.Errorf("a plain task was counted as an allowance: %+v", report.Allowances)
	}
}
