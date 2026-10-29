package spend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func creditOf(usd float64) CreditLimit { return func() (float64, bool) { return usd, true } }

// The credit is held across months and across the organization's own keys: only the operator's keys use it, and once it is
// used a call on them is refused with what to do, while one on the organization's own key goes.
func TestACreditIsUsedOnceByTheOperatorsKeysAndSaysHowToCarryOn(t *testing.T) {
	ledger := fixedLedger(september)
	if err := ledger.UseCredit(creditOf(5), filepath.Join(t.TempDir(), "credit-used.json")); err != nil {
		t.Fatal(err)
	}
	ledger.Begin("a-job", Terms{})
	ledger.Count("a-job", answered(3, september.AddDate(0, -1, 0)))
	own := answered(10, september)
	own.Own = true
	ledger.Count("a-job", own)
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal != nil {
		t.Fatalf("refused with credit left: %+v", refusal)
	}
	ledger.Count("a-job", answered(2, september))
	refusal := ledger.Allow("a-job", "", "fixture-model", false)
	if refusal == nil || refusal.Cap != "credit" || refusal.SpentUSD != 5 || refusal.LimitUSD != 5 || !strings.Contains(refusal.Reason, "own model key in Admin, Models") {
		t.Fatalf("a call past the credit: %+v", refusal)
	}
	if refusal := ledger.Allow("a-job", "", "fixture-model", true); refusal != nil {
		t.Errorf("a call on the organization's own key was held to the operator's credit: %+v", refusal)
	}
	if used, limit, held := ledger.Credit(); !held || used != 5 || limit != 5 {
		t.Errorf("%v %v %v", used, limit, held)
	}
}

// What was used is kept in a file that only grows, so a server that starts again after records were deleted does not give any
// of the credit back.
func TestWhatWasUsedOfTheCreditOutlivesTheRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credit-used.json")
	first := fixedLedger(september)
	if err := first.UseCredit(creditOf(5), path); err != nil {
		t.Fatal(err)
	}
	first.Begin("a-job", Terms{})
	first.Count("a-job", answered(4, september))
	second := fixedLedger(september)
	if err := second.UseCredit(creditOf(5), path); err != nil {
		t.Fatal(err)
	}
	if used, _, _ := second.Credit(); used != 4 {
		t.Fatalf("after the records went the credit used is %v", used)
	}
	second.Begin("b-job", Terms{})
	second.Count("b-job", answered(1, september))
	if refusal := second.Allow("b-job", "", "fixture-model", false); refusal == nil || refusal.Cap != "credit" {
		t.Errorf("%+v", refusal)
	}
	// A server that starts again with its records still there counts them once.
	third := fixedLedger(september)
	third.Begin("a-job", Terms{})
	third.Count("a-job", answered(4, september))
	third.Count("a-job", answered(1, september))
	if err := third.UseCredit(creditOf(10), path); err != nil {
		t.Fatal(err)
	}
	if used, _, _ := third.Credit(); used != 5 {
		t.Errorf("the records that are still there were counted twice: %v", used)
	}
	os.WriteFile(path, []byte("not json"), 0o600)
	if err := fixedLedger(september).UseCredit(creditOf(5), path); err == nil {
		t.Error("a file that cannot be read was taken as nothing used")
	}
}

func TestACreditLimitThatCannotBeReadAllowsNothingOfTheOperatorsAndAnUnpricedModelIsRefused(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.UseCredit(creditOf(0), filepath.Join(t.TempDir(), "credit-used.json"))
	ledger.Begin("a-job", Terms{})
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal == nil || refusal.Cap != "credit" {
		t.Errorf("a credit of nothing let a call go: %+v", refusal)
	}
	ledger.UseCredit(creditOf(5), filepath.Join(t.TempDir(), "credit-used.json"))
	if refusal := ledger.Allow("a-job", "", "unpriced-model", false); refusal == nil || refusal.Cap != "credit" || !strings.Contains(refusal.Reason, "no price") {
		t.Errorf("a model the credit cannot count went: %+v", refusal)
	}
	none := fixedLedger(september)
	none.UseCredit(func() (float64, bool) { return 0, false }, "")
	none.Begin("a-job", Terms{})
	if refusal := none.Allow("a-job", "", "fixture-model", false); refusal != nil {
		t.Errorf("with no credit there is no limit: %+v", refusal)
	}
}
