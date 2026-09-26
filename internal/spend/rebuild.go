package spend

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/getlatentic/haas/internal/job"
)

// Rebuild reads every job under root back into a ledger, so a server that
// restarts in the middle of a month still knows what each team has spent. The
// lines in each job's out/proxy.jsonl are the ledger; this is a cache of them.
func Rebuild(root string, policy Policy) (*Ledger, error) {
	ledger := NewLedger(policy)
	listing, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return ledger, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range listing {
		if !entry.IsDir() {
			continue
		}
		dir := job.Dir(filepath.Join(root, entry.Name()))
		spec, err := job.Read(dir)
		if err != nil {
			continue
		}
		ledger.Begin(entry.Name(), TermsOf(spec))
		calls, err := ReadCalls(dir.OutFile("proxy.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, call := range calls {
			ledger.Count(entry.Name(), call)
		}
	}
	return ledger, nil
}

// TermsOf is what a job's spec allows it to spend, and where it is reported.
// A job's project is the repository it worked in: the one thing every job
// already names that a platform lead would group spend by.
func TermsOf(spec job.Spec) Terms {
	return Terms{Team: spec.Team, Project: spec.Repo, CapUSD: spec.Budget.USD}
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
