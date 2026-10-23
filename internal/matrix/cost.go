package matrix

import (
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/spend"
)

// Where a run's cost was read from.
const (
	// FromProxy is the proxy's record of what the provider answered: the
	// count, rather than a claim.
	FromProxy = "proxy"
	// FromHarness is the agent's own report, read off its output into the
	// manifest. It is what a run that reached the model directly has.
	FromHarness = "harness"
)

// spent is what a run cost and the tokens it was billed. A null stays null: a
// run that reported nothing is unmeasured, never free.
type spent struct {
	USD    *float64
	Tokens *int64
	// Unpriced counts calls the proxy saw and could not price. While it is
	// above zero, USD is a floor rather than a total.
	Unpriced int
	From     string
}

// spentBy reads the proxy's record when the run went through the proxy, and
// the harness's report when it did not.
func spentBy(dir job.Dir, manifest evidence.Manifest) (spent, error) {
	calls, err := spend.ReadCalls(dir.OutFile("proxy.jsonl"))
	if err != nil {
		return spent{}, err
	}
	tally := counted(calls)
	if tally.Calls == 0 {
		return reported(manifest), nil
	}
	if tally.Unmeasured == tally.Calls {
		return spent{Unpriced: tally.Unmeasured, From: FromProxy}, nil
	}
	tokens := tally.InputTokens + tally.OutputTokens
	return spent{USD: &tally.USD, Tokens: &tokens, Unpriced: tally.Unmeasured, From: FromProxy}, nil
}

// reported is the harness's own account, and nothing where it gave none.
func reported(manifest evidence.Manifest) spent {
	if manifest.CostUSD == nil && manifest.Tokens == nil {
		return spent{}
	}
	return spent{USD: manifest.CostUSD, Tokens: manifest.Tokens, From: FromHarness}
}

// counted adds the calls up in a ledger of their own, so a cell's cost is
// counted by the same rules as the spend report and a team's cap.
func counted(calls []spend.Call) spend.Tally {
	ledger := spend.NewLedger(spend.Policy{})
	for _, call := range calls {
		ledger.Count("run", call)
	}
	var total spend.Tally
	for _, month := range ledger.Months() {
		for _, line := range ledger.Report(month).Jobs {
			total.Calls += line.Calls
			total.USD += line.USD
			total.Unmeasured += line.Unmeasured
			total.InputTokens += line.InputTokens
			total.OutputTokens += line.OutputTokens
		}
	}
	return total
}
