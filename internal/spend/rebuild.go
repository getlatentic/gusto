package spend

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/getlatentic/haas/internal/job"
)

// Rebuild reads every job under root back into a ledger, so a server that
// restarts in the middle of a month still knows what each team has spent. The
// lines in each job's out/proxy.jsonl are the ledger; this is a cache of them.
func Rebuild(root string, policy Policy) (*Ledger, error) {
	return RebuildWhere(root, policy, nil)
}

// RebuildWhere reads back only the jobs keep names, which is how a person's own
// spend is told apart from everybody's. A nil keep reads them all.
func RebuildWhere(root string, policy Policy, keep func(job string) bool) (*Ledger, error) {
	ledger := NewLedger(policy)
	listing, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return ledger, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range listing {
		if !entry.IsDir() || (keep != nil && !keep(entry.Name())) {
			continue
		}
		dir := job.Dir(filepath.Join(root, entry.Name()))
		spec, err := job.Read(dir)
		if err != nil {
			continue
		}
		ledger.Begin(entry.Name(), TermsOf(spec))
		calls, err := dated(dir.OutFile("proxy.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, call := range calls {
			ledger.Count(entry.Name(), policy.reprice(call))
		}
	}
	return ledger, nil
}

// reprice prices a call the proxy counted before its model had a price. The
// tokens are the provider's and the price is the operator's, so a price added
// later applies to what was already counted; a line that carried a cost keeps
// it. The evidence on disk is not rewritten.
func (p Policy) reprice(call Call) Call {
	if call.Measured() || !call.Counted() || call.Refused != nil {
		return call
	}
	usage := &Usage{Input: *call.InputTokens, Cached: valueOf(call.CachedTokens), Output: *call.OutputTokens}
	call.USD = p.Cost(call.Credential, call.Model, usage, time.Unix(call.AtUnix, 0))
	return call
}

// dated is a job's calls with a time on every one. The proxy wrote lines
// without one before it recorded when a call was sent, and those are dated by
// when the file was last written, which is within the job's own run, rather
// than filed under 1970.
func dated(path string) ([]Call, error) {
	calls, err := ReadCalls(path)
	if err != nil || len(calls) == 0 {
		return calls, err
	}
	written, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	for i := range calls {
		if calls[i].AtUnix == 0 {
			calls[i].AtUnix = written.ModTime().Unix()
		}
	}
	return calls, nil
}

// TermsOf is what a job's spec allows it to spend, and where it is reported.
// A job's project is the repository it worked in: the one thing every job
// already names that a platform lead would group spend by.
func TermsOf(spec job.Spec) Terms {
	return Terms{Team: spec.Team, Project: spec.Repo, Harness: spec.Harness, CapUSD: spec.Budget.USD}
}

// ReadCalls is every call a job's proxy record holds. A missing file is a job
// that never reached the proxy. A line that is not a call is passed over and
// said, because a server killed mid-write leaves half a line and refusing to
// start over it would stop every other job's accounting too.
func ReadCalls(path string) ([]Call, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var calls []Call
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for lines.Scan() {
		var call Call
		if err := json.Unmarshal(lines.Bytes(), &call); err != nil {
			slog.Warn("a line of the proxy's record is not a call", "file", path, "error", err)
			continue
		}
		calls = append(calls, call)
	}
	return calls, lines.Err()
}

// Stopped is the refusal that stopped a run that started at since, read from
// the job's proxy record, or nil when no cap refused it anything. The record
// outlives a run, so a refusal from before since belongs to an earlier one. The
// first refusal is the one that stopped it; any after are the harness asking
// again.
func Stopped(path string, since int64) (*Refusal, error) {
	calls, err := ReadCalls(path)
	if err != nil {
		return nil, err
	}
	for _, call := range calls {
		if call.Refused != nil && call.AtUnix >= since {
			return call.Refused, nil
		}
	}
	return nil, nil
}
