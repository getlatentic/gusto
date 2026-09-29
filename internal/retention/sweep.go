package retention

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/getlatentic/haas/internal/jobs"
)

// Jobs is what a sweep reads jobs from and removes them through. The server
// gives its index, so what it holds in memory follows what is on disk; a
// dry run over Scan never removes anything.
type Jobs interface {
	List() []jobs.Record
	Remove(id string) error
}

// Kind is what a sweep removed of a job.
const (
	Workspace = "workspace"
	Record    = "record"
)

// Action is one removal, made or, in a dry run, planned.
type Action struct {
	Job   string `json:"job"`
	Brief string `json:"brief"`
	Kind  string `json:"kind"`
	State string `json:"state"`
	// Days is how long ago the job ended.
	Days  int   `json:"days"`
	Bytes int64 `json:"bytes"`
	// Failed is why a removal did not happen.
	Failed string `json:"failed,omitempty"`
}

// Report is what a sweep did.
type Report struct {
	At      int64    `json:"at"`
	Dry     bool     `json:"dry"`
	Actions []Action `json:"actions"`
	// Held is the jobs the policy would have removed but a hold kept.
	Held  []string `json:"held,omitempty"`
	Freed int64    `json:"freed"`
}

// Removed counts the actions of one kind that happened.
func (r Report) Removed(kind string) int {
	n := 0
	for _, action := range r.Actions {
		if action.Kind == kind && action.Failed == "" {
			n++
		}
	}
	return n
}

// Sweep removes what the policy says is due. It touches only jobs that ended
// and that no agent owns: a job on an agent belongs to that agent's
// conversation, and goes when the agent does. A job with a hold is skipped, and
// so is one asked for by somebody with a hold.
func Sweep(all Jobs, root string, policy Policy, holds Held, now time.Time, dry bool) Report {
	report := Report{At: now.Unix(), Dry: dry}
	for _, record := range all.List() {
		if !record.State.Done() || record.Instance != "" {
			continue
		}
		days := daysSince(record, now)
		kind := due(policy, record, days)
		if kind == "" {
			continue
		}
		if _, held := holds.Covers(record); held {
			report.Held = append(report.Held, record.ID)
			continue
		}
		action := Action{Job: record.ID, Brief: record.Brief, Kind: kind, State: string(record.State), Days: days}
		target := filepath.Join(root, record.ID)
		if kind == Workspace {
			target = filepath.Join(target, "workspace")
		}
		action.Bytes = size(target)
		if kind == Workspace && action.Bytes == 0 {
			continue
		}
		if !dry {
			if err := remove(all, record, kind, target); err != nil {
				action.Failed = err.Error()
			} else {
				report.Freed += action.Bytes
			}
		}
		report.Actions = append(report.Actions, action)
	}
	sort.Strings(report.Held)
	return report
}

// due is what the policy says a job has outlived, the record first because it
// takes the workspace with it.
func due(policy Policy, record jobs.Record, days int) string {
	state := string(record.State)
	if limit := policy.RecordDays[state]; limit > 0 && days >= limit {
		return Record
	}
	if limit := policy.WorkspaceDays[state]; limit > 0 && days >= limit {
		return Workspace
	}
	return ""
}

func remove(all Jobs, record jobs.Record, kind, target string) error {
	if kind == Record {
		return all.Remove(record.ID)
	}
	return os.RemoveAll(target)
}

// daysSince is whole days since the job ended, or was queued when it has no end.
func daysSince(record jobs.Record, now time.Time) int {
	at := record.Ended
	if at == 0 {
		at = record.Queued
	}
	return int(now.Sub(time.Unix(at, 0)) / (24 * time.Hour))
}

// size is the bytes under a path, counting what is there and no more.
func size(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// Scanned is a read-only view of the jobs on disk, for a dry run from the
// command line while a server runs.
type Scanned struct{ Root string }

func (s Scanned) List() []jobs.Record {
	all, _ := jobs.Scan(s.Root)
	return all
}

func (Scanned) Remove(id string) error { return fmt.Errorf("a dry run removes nothing: %s", id) }
