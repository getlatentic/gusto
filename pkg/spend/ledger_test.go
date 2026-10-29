package spend

import (
	"math"
	"strings"
	"testing"
	"time"
)

func dollars(amount float64) *float64 { return &amount }

func tokens(count int64) *int64 { return &count }

// answered is a call the provider answered and the proxy priced.
func answered(cost float64, at time.Time) Call {
	return Call{Path: "/v1/chat/completions", Status: 200, AtUnix: at.Unix(), Model: "fixture-model",
		InputTokens: tokens(10), OutputTokens: tokens(5), USD: dollars(cost)}
}

func fixedLedger(at time.Time) *Ledger {
	ledger := NewLedger(fixturePolicy())
	ledger.now = func() time.Time { return at }
	return ledger
}

var september = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func TestAJobCapRefusesOnceItIsSpent(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("a-job", Terms{CapUSD: dollars(0.5)})
	ledger.Count("a-job", answered(0.3, september))
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal != nil {
		t.Fatalf("refused under the cap: %+v", refusal)
	}
	ledger.Count("a-job", answered(0.3, september))
	refusal := ledger.Allow("a-job", "", "fixture-model", false)
	if refusal == nil || refusal.Cap != "job" || refusal.LimitUSD != 0.5 || refusal.SpentUSD != 0.6 {
		t.Fatalf("refusal %+v", refusal)
	}
}

func TestAZeroCapAllowsNothingAndNoCapAllowsEverything(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("nothing", Terms{CapUSD: dollars(0)})
	ledger.Begin("anything", Terms{})
	if ledger.Allow("nothing", "", "fixture-model", false) == nil {
		t.Fatal("a cap of zero let a call go")
	}
	ledger.Count("anything", answered(1000, september))
	if refusal := ledger.Allow("anything", "", "unpriced-model", false); refusal != nil {
		t.Fatalf("a job with no cap was refused: %+v", refusal)
	}
}

// A team's cap is shared by every job that names the team, and it is a cap
// for the month: a new month starts from nothing.
func TestATeamCapStopsTheJobThatWouldExceedIt(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("first", Terms{Team: "platform"})
	ledger.Begin("second", Terms{Team: "platform"})
	ledger.Begin("elsewhere", Terms{Team: "research"})
	ledger.Count("first", answered(0.6, september))
	ledger.Count("second", answered(0.5, september))
	refusal := ledger.Allow("second", "", "fixture-model", false)
	if refusal == nil || refusal.Cap != "team" || refusal.Team != "platform" {
		t.Fatalf("refusal %+v", refusal)
	}
	if !strings.Contains(refusal.Reason, "team platform") || !strings.Contains(refusal.Reason, "2026-09") {
		t.Fatalf("the reason should name the team and the month: %q", refusal.Reason)
	}
	if refusal := ledger.Allow("elsewhere", "", "fixture-model", false); refusal != nil {
		t.Fatalf("another team was refused: %+v", refusal)
	}
	ledger.now = func() time.Time { return september.AddDate(0, 1, 0) }
	if refusal := ledger.Allow("second", "", "fixture-model", false); refusal != nil {
		t.Fatalf("a new month was refused: %+v", refusal)
	}
}

// A cap that cannot see what is spent is not holding anything.
func TestACapRefusesWhatItCannotCount(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("a-job", Terms{CapUSD: dollars(5)})
	refusal := ledger.Allow("a-job", "", "unpriced-model", false)
	if refusal == nil || !strings.Contains(refusal.Reason, "unpriced-model has no price") {
		t.Fatalf("an unpriced model under a cap: %+v", refusal)
	}
	blind := answered(0, september)
	blind.InputTokens, blind.OutputTokens, blind.USD = nil, nil, nil
	ledger.Count("a-job", blind)
	refusal = ledger.Allow("a-job", "", "fixture-model", false)
	if refusal == nil || !strings.Contains(refusal.Reason, "carried no usage") {
		t.Fatalf("after an answer with no usage: %+v", refusal)
	}
}

// A provider that refused a call billed nothing for it, so an error without
// usage costs nothing and does not blind the cap.
func TestAnErrorWithoutUsageDoesNotStopACappedJob(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("a-job", Terms{CapUSD: dollars(5)})
	ledger.Count("a-job", Call{Path: "/v1/chat/completions", Status: 429, AtUnix: september.Unix(), Model: "fixture-model"})
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal != nil {
		t.Fatalf("refused after a 429: %+v", refusal)
	}
}

func TestARefusedCallCostsNothingAndIsNotACall(t *testing.T) {
	var tally Tally
	tally.add(RefusedCall("/v1/chat/completions", "fixture-model", september.Unix(), &Refusal{Cap: "job"}))
	unmeasured := Call{Status: 200, Model: "fixture-model"}
	tally.add(unmeasured)
	tally.add(answered(0.25, september))
	if tally.Calls != 2 || tally.Unmeasured != 1 || tally.USD != 0.25 || tally.InputTokens != 10 {
		t.Fatalf("tally %+v", tally)
	}
}

// A request to a host other than the model spends no model money, and a
// refused one is not an unmeasured call a cap should stop on.
func TestARequestToAnotherHostIsNotSpend(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("a-job", Terms{CapUSD: dollars(5)})
	ledger.Count("a-job", Call{Host: "pastebin.com:443", Status: 403, Denied: "not on the allowlist", AtUnix: september.Unix()})
	ledger.Count("a-job", Call{Host: "registry.npmjs.org:443", Status: 200, AtUnix: september.Unix()})
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal != nil {
		t.Fatalf("refused after requests to other hosts: %+v", refusal)
	}
	if calls := ledger.jobs["a-job"].tally.Calls; calls != 0 {
		t.Fatalf("%d calls counted as spend", calls)
	}
}

// A model nobody priced still used tokens, and the provider still said how
// many. The count is a fact whether or not the money is known.
func TestTokensAreCountedWhenTheModelHasNoPrice(t *testing.T) {
	ledger := NewLedger(Policy{})
	in, cached, out := int64(900), int64(600), int64(40)
	ledger.Count("a-job", Call{Path: "/v1/chat/completions", Status: 200, AtUnix: 1790496257,
		Model: "deepseek-flash", Credential: "deepseek", InputTokens: &in, CachedTokens: &cached, OutputTokens: &out})
	ledger.Count("a-job", Call{Path: "/v1/chat/completions", Status: 200, AtUnix: 1790496258, Model: "deepseek-flash"})
	report := ledger.Report("2026-09")
	total := report.Total
	if total.InputTokens != 900 || total.CachedTokens != 600 || total.OutputTokens != 40 {
		t.Errorf("tokens %+v", total)
	}
	if total.Unmeasured != 2 || total.Uncounted != 1 {
		t.Errorf("unmeasured %d, uncounted %d", total.Unmeasured, total.Uncounted)
	}
	if len(report.Models) != 2 || len(report.Days) != 1 || report.Days[0].Name != "2026-09-27" {
		t.Errorf("models %+v, days %+v", report.Models, report.Days)
	}
}

// A provider bills nothing for a call it turns away, or for listing its
// models, and says nothing of usage for either. Those cost nothing; they are
// not calls whose cost is unknown.
func TestACallTheProviderRefusedCostsNothing(t *testing.T) {
	ledger := NewLedger(Policy{})
	ledger.Count("a-job", Call{Path: "/v1/chat/completions", Status: 400, AtUnix: 1790496257, Model: "deepseek-flash"})
	ledger.Count("a-job", Call{Path: "/v1/models", Status: 200, AtUnix: 1790496258, Credential: "deepseek"})
	total := ledger.Report("2026-09").Total
	if total.Calls != 2 || total.Unmeasured != 0 || total.Uncounted != 0 {
		t.Errorf("total %+v", total)
	}
}

func TestPricesChangedWhileRunningApplyToTheNextCallAndNotToOnesAlreadyCosted(t *testing.T) {
	ledger := NewLedger(Policy{Prices: map[string]Price{"a/m": {InputPerMillion: 1, OutputPerMillion: 1}}})
	usage := &Usage{Input: 1_000_000}
	before := ledger.Policy().Cost("a", "m", usage, time.Unix(0, 0))
	ledger.SetPrices(map[string]Price{"a/m": {InputPerMillion: 5, OutputPerMillion: 5}})
	after := ledger.Policy().Cost("a", "m", usage, time.Unix(0, 0))
	if before == nil || after == nil || *before != 1 || *after != 5 {
		t.Errorf("%v then %v", before, after)
	}
	if ledger.Policy().Priced("a", "gone") {
		t.Error("a model with no price is priced")
	}
}

func organizationLedger(at time.Time, cap float64) *Ledger {
	policy := fixturePolicy()
	policy.Organization = &TeamCap{USDPerMonth: cap}
	ledger := NewLedger(policy)
	ledger.now = func() time.Time { return at }
	return ledger
}

func TestAnOrganizationCapStopsEveryJobOnceTheJobsTogetherHaveSpentIt(t *testing.T) {
	ledger := organizationLedger(september, 0.5)
	ledger.Begin("first", Terms{Team: "red"})
	ledger.Begin("second", Terms{Team: "blue"})
	ledger.Begin("third", Terms{})
	ledger.Count("first", answered(0.3, september))
	ledger.Count("second", answered(0.15, september))
	for _, job := range []string{"first", "second", "third"} {
		if refusal := ledger.Allow(job, "", "fixture-model", false); refusal != nil {
			t.Fatalf("%s was refused under the cap: %+v", job, refusal)
		}
	}
	ledger.Count("second", answered(0.1, september))
	for _, job := range []string{"first", "second", "third"} {
		refusal := ledger.Allow(job, "", "fixture-model", false)
		if refusal == nil || refusal.Cap != "organisation" || refusal.LimitUSD != 0.5 || math.Abs(refusal.SpentUSD-0.55) > 1e-9 || !strings.Contains(refusal.Reason, "this organization has spent") {
			t.Errorf("%s: %+v", job, refusal)
		}
	}
}

func TestAnOrganizationCapIsForAMonthAndAJobCapStillComesFirst(t *testing.T) {
	ledger := organizationLedger(september, 0.5)
	ledger.Begin("old", Terms{})
	ledger.Count("old", answered(0.6, september.AddDate(0, -1, 0)))
	if refusal := ledger.Allow("old", "", "fixture-model", false); refusal != nil {
		t.Errorf("last month's spending counted: %+v", refusal)
	}
	ledger.Begin("capped", Terms{CapUSD: dollars(0.1)})
	ledger.Count("capped", answered(0.2, september))
	if refusal := ledger.Allow("capped", "", "fixture-model", false); refusal == nil || refusal.Cap != "job" {
		t.Errorf("the job's own cap is not the one named: %+v", refusal)
	}
}

func TestAnOrganizationCapFailsClosedOnAModelItCannotPrice(t *testing.T) {
	ledger := organizationLedger(september, 0.5)
	ledger.Begin("a-job", Terms{})
	refusal := ledger.Allow("a-job", "", "unpriced-model", false)
	if refusal == nil || refusal.Cap != "organisation" || !strings.Contains(refusal.Reason, "no price in the spend policy") {
		t.Errorf("a call that cannot be counted was let go: %+v", refusal)
	}
}

// A key the organization added for itself is its own to pay for: the operator's cap does not count its calls and does not stop
// them, so an organization whose free credit is spent carries on with its own key.
func TestTheOperatorsCapCountsOnlyTheOperatorsKeys(t *testing.T) {
	ledger := organizationLedger(september, 0.5)
	ledger.Begin("a-job", Terms{})
	own := answered(2.0, september)
	own.Own = true
	ledger.Count("a-job", own)
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal != nil {
		t.Fatalf("the organization's own spending counted against the operator's cap: %+v", refusal)
	}
	ledger.Count("a-job", answered(0.6, september))
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal == nil || refusal.Cap != "organisation" || math.Abs(refusal.SpentUSD-0.6) > 1e-9 {
		t.Fatalf("a call on the operator's key went past the cap: %+v", refusal)
	}
	if refusal := ledger.Allow("a-job", "", "fixture-model", true); refusal != nil {
		t.Errorf("a call on the organization's own key was stopped by the operator's cap: %+v", refusal)
	}
	if refusal := ledger.Allow("a-job", "", "unpriced-model", true); refusal != nil {
		t.Errorf("the operator's cap held back an unpriced model on the organization's own key: %+v", refusal)
	}
	if got := ledger.SpentThisMonth(ByPayer, PaidByOrganization); got != 2.0 {
		t.Errorf("the organization's own spending is %v", got)
	}
	if got := ledger.SpentThisMonth(ByPayer, PaidByOperator); got != 0.6 {
		t.Errorf("the operator's is %v", got)
	}
}

// A team's cap is the organization's own, and counts every call whoever's key it went on.
func TestATeamCapCountsTheOrganizationsOwnKeysToo(t *testing.T) {
	policy := fixturePolicy()
	policy.Teams = map[string]TeamCap{"red": {USDPerMonth: 1}}
	ledger := NewLedger(policy)
	ledger.now = func() time.Time { return september }
	ledger.Begin("a-job", Terms{Team: "red"})
	own := answered(1.5, september)
	own.Own = true
	ledger.Count("a-job", own)
	if refusal := ledger.Allow("a-job", "", "fixture-model", true); refusal == nil || refusal.Cap != "team" {
		t.Errorf("%+v", refusal)
	}
}

func TestNoOrganizationCapIsNoLimit(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("a-job", Terms{})
	ledger.Count("a-job", answered(1000, september))
	if refusal := ledger.Allow("a-job", "", "fixture-model", false); refusal != nil {
		t.Errorf("%+v", refusal)
	}
}
