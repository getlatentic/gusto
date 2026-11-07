package runner

import (
	"context"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/task"
)

// ending is what a run knows once its agent and its check are done, which is what its record says. A run in this process
// and a run in a box end the same way.
type ending struct {
	dir task.Dir
	// since is when the agent started, which the caps and the model's silences are counted from.
	since    int64
	verdict  *evidence.Verdict
	exit     int
	harness  string
	digest   string
	version  string
	session  string
	endpoint string
	tokens   *int64
	cost     *float64
	signing  signing.Identity
}

// finished closes the agent's record and says what it spent. An error closing it is the run's when the run had none.
func finished(record *Record, err error) (*int64, *float64, error) {
	tokens, cost := record.Spent()
	if closeErr := record.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return tokens, cost, err
}

// close writes the run's manifest over everything in out/, seals it when a key is configured, and returns the exit code the
// run earned. ctx is the run's own, which says whether the budget stopped it.
func (e ending) close(ctx context.Context) (int, error) {
	capped, err := stoppedByCap(e.dir, e.since)
	if err != nil {
		return 0, err
	}
	silent, err := unanswered(e.dir, e.since)
	if err != nil {
		return 0, err
	}
	spent, files, err := evidence.Of(e.dir)
	if err != nil {
		return 0, err
	}
	if err := evidence.Write(e.dir, evidence.Manifest{
		Task:            spent,
		Files:           files,
		Check:           e.verdict,
		Harness:         e.harness,
		HarnessDigest:   e.digest,
		HarnessVersion:  e.version,
		Session:         e.session,
		HarnessExit:     e.exit,
		StoppedAtBudget: ctx.Err() == context.DeadlineExceeded,
		StoppedBy:       stoppedBy(ctx, e.dir),
		StoppedByCap:    capped,
		ModelUnanswered: silent,
		ModelEndpoint:   e.endpoint,
		Tokens:          e.tokens,
		CostUSD:         e.cost,
		Signer:          e.signing.Says(),
		AtUnix:          evidence.Now(),
	}); err != nil {
		return 0, err
	}
	if e.signing.Signed() {
		if err := evidence.Seal(e.dir, e.signing.Key); err != nil {
			return 0, err
		}
	}
	return evidence.Earned(e.verdict, e.exit, silent != nil), nil
}
