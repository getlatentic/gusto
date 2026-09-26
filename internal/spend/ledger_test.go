package spend

import (
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
	if refusal := ledger.Allow("a-job", "fixture-model"); refusal != nil {
		t.Fatalf("refused under the cap: %+v", refusal)
	}
	ledger.Count("a-job", answered(0.3, september))
	refusal := ledger.Allow("a-job", "fixture-model")
	if refusal == nil || refusal.Cap != "job" || refusal.LimitUSD != 0.5 || refusal.SpentUSD != 0.6 {
		t.Fatalf("refusal %+v", refusal)
	}
}

func TestAZeroCapAllowsNothingAndNoCapAllowsEverything(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("nothing", Terms{CapUSD: dollars(0)})
	ledger.Begin("anything", Terms{})
	if ledger.Allow("nothing", "fixture-model") == nil {
		t.Fatal("a cap of zero let a call go")
	}
	ledger.Count("anything", answered(1000, september))
	if refusal := ledger.Allow("anything", "unpriced-model"); refusal != nil {
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
	refusal := ledger.Allow("second", "fixture-model")
	if refusal == nil || refusal.Cap != "team" || refusal.Team != "platform" {
		t.Fatalf("refusal %+v", refusal)
	}
	if !strings.Contains(refusal.Reason, "team platform") || !strings.Contains(refusal.Reason, "2026-09") {
		t.Fatalf("the reason should name the team and the month: %q", refusal.Reason)
	}
	if refusal := ledger.Allow("elsewhere", "fixture-model"); refusal != nil {
		t.Fatalf("another team was refused: %+v", refusal)
	}
	ledger.now = func() time.Time { return september.AddDate(0, 1, 0) }
	if refusal := ledger.Allow("second", "fixture-model"); refusal != nil {
		t.Fatalf("a new month was refused: %+v", refusal)
	}
}

// A cap that cannot see what is spent is not holding anything.
func TestACapRefusesWhatItCannotCount(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("a-job", Terms{CapUSD: dollars(5)})
	refusal := ledger.Allow("a-job", "unpriced-model")
	if refusal == nil || !strings.Contains(refusal.Reason, "unpriced-model has no price") {
		t.Fatalf("an unpriced model under a cap: %+v", refusal)
	}
	blind := answered(0, september)
	blind.InputTokens, blind.OutputTokens, blind.USD = nil, nil, nil
	ledger.Count("a-job", blind)
	refusal = ledger.Allow("a-job", "fixture-model")
	if refusal == nil || !strings.Contains(refusal.Reason, "carried no usage") {
		t.Fatalf("after an answer with no usage: %+v", refusal)
	}
}

// A provider that refused a call billed nothing for it, so an error without
// usage is recorded as unmeasured and does not blind the cap.
func TestAnErrorWithoutUsageDoesNotStopACappedJob(t *testing.T) {
	ledger := fixedLedger(september)
	ledger.Begin("a-job", Terms{CapUSD: dollars(5)})
	ledger.Count("a-job", Call{Path: "/v1/chat/completions", Status: 429, AtUnix: september.Unix(), Model: "fixture-model"})
	if refusal := ledger.Allow("a-job", "fixture-model"); refusal != nil {
		t.Fatalf("refused after a 429: %+v", refusal)
	}
}

func TestARefusedCallCostsNothingAndIsNotACall(t *testing.T) {
	var tally Tally
	tally.add(RefusedCall("/v1/chat/completions", "fixture-model", september.Unix(), &Refusal{Cap: "job"}))
	unmeasured := Call{Status: 200}
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
	if refusal := ledger.Allow("a-job", "fixture-model"); refusal != nil {
		t.Fatalf("refused after requests to other hosts: %+v", refusal)
	}
	if calls := ledger.jobs["a-job"].tally.Calls; calls != 0 {
		t.Fatalf("%d calls counted as spend", calls)
	}
}
