package spend

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

// budgets is the budgets an admin set, by person, which a test changes as an admin would.
type budgets struct {
	mu  sync.Mutex
	usd map[string]float64
}

func (b *budgets) set(person string, usd float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.usd[person] = usd
}

func (b *budgets) of(person string) (float64, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	usd, found := b.usd[person]
	return usd, found
}

func personLedger(prices map[string]Price) (*Ledger, *budgets) {
	ledger := NewLedger(Policy{Prices: prices})
	held := &budgets{usd: map[string]float64{}}
	ledger.UsePersonBudgets(held.of)
	return ledger, held
}

var oneDollar = func() *float64 { usd := 1.0; return &usd }()

func dollarCall() Call {
	return Call{Path: "/v1/chat/completions", Model: "m", AtUnix: time.Now().Unix(), Status: 200, USD: oneDollar}
}

// A person is held to their month's budget across all their tasks, and nobody else is held to it.
func TestAPersonIsHeldToTheirBudgetAcrossTheirTasks(t *testing.T) {
	ledger, held := personLedger(map[string]Price{"m": {InputPerMillion: 1}})
	held.set("ada@acme.com", 2)
	for _, task := range []string{"t1", "t2"} {
		ledger.Begin(task, TermsOf(job.Spec{Harness: "pi", Person: "Ada@Acme.com"}))
		if refusal := ledger.Allow(task, "", "m", false); refusal != nil {
			t.Fatalf("%s was refused before the budget was spent: %+v", task, refusal)
		}
		ledger.Count(task, dollarCall())
	}
	ledger.Begin("t3", TermsOf(job.Spec{Harness: "pi", Person: "Ada@Acme.com"}))
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

// A budget an admin lowers holds from the next call of a task already running, and from a task the server started again.
func TestALoweredBudgetStopsATaskAlreadyRunning(t *testing.T) {
	ledger, held := personLedger(map[string]Price{"m": {InputPerMillion: 1}})
	held.set("ada@acme.com", 10)
	ledger.Begin("t1", TermsOf(job.Spec{Harness: "pi", Person: "ada@acme.com"}))
	ledger.Count("t1", dollarCall())
	held.set("ada@acme.com", 1)
	if refusal := ledger.Allow("t1", "", "m", false); refusal == nil || refusal.Cap != "person" || refusal.LimitUSD != 1 {
		t.Fatalf("the budget as it was when the task started held: %+v", refusal)
	}
	held.set("ada@acme.com", 0)
	ledger.Begin("t2", TermsOf(job.Spec{Harness: "pi", Person: "ada@acme.com"}))
	if refusal := ledger.Allow("t2", "", "m", false); refusal == nil || refusal.Cap != "person" {
		t.Errorf("a budget of $0 let a call leave: %+v", refusal)
	}
}

// A person's budget, or a room agent's allowance, that is the only cap on a task cannot count a model with no price, so it
// refuses the call, as a task's own cap does.
func TestABudgetOrAnAllowanceAloneRefusesWhatItCannotCount(t *testing.T) {
	ledger, held := personLedger(map[string]Price{"m": {InputPerMillion: 1}})
	held.set("ada@acme.com", 10)
	ledger.Begin("t1", TermsOf(job.Spec{Harness: "pi", Person: "ada@acme.com"}))
	if refusal := ledger.Allow("t1", "", "unpriced", false); refusal == nil || refusal.Cap != "person" || !strings.Contains(refusal.Reason, "no price") {
		t.Errorf("an unpriced call passed under a person's budget: %+v", refusal)
	}
	ten := 10.0
	ledger.Begin("t2", TermsOf(job.Spec{Harness: "pi", Session: "room", Agent: "a1", Budget: job.Budget{MonthUSD: &ten}}))
	if refusal := ledger.Allow("t2", "", "unpriced", false); refusal == nil || refusal.Cap != "allowance" {
		t.Errorf("an unpriced call passed under an agent's allowance: %+v", refusal)
	}
	ledger.Begin("t3", TermsOf(job.Spec{Harness: "pi", Person: "bob@acme.com"}))
	if refusal := ledger.Allow("t3", "", "unpriced", false); refusal != nil {
		t.Errorf("a task no cap governs was refused an unpriced call: %+v", refusal)
	}
}
