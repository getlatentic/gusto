package retention

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/getlatentic/gusto/internal/tasks"
)

// Tasks is what a sweep reads tasks from and removes them through. The server
// gives its index, so what it holds in memory follows what is on disk; a
// dry run over Scan never removes anything.
type Tasks interface {
	List() []tasks.Record
	Remove(id string) error
}

// Kind is what a sweep removed of a task.
const (
	WorkingFolder = "workspace"
	Record        = "record"
)

// Action is one removal, made or, in a dry run, planned.
type Action struct {
	Task  string `json:"job"`
	Brief string `json:"brief"`
	Kind  string `json:"kind"`
	State string `json:"state"`
	// Days is how long ago the task ended.
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
	// Held is the tasks the policy would have removed but a hold kept.
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

// Sweep removes what the policy says is due. It touches only tasks that ended, and not a turn of a room that live says is still
// kept: that turn belongs to the room's conversation. A turn of a room that was removed is swept like any task. A task with a
// hold is skipped, and so is one asked for by somebody with a hold; holds says so at the moment of each removal.
func Sweep(all Tasks, root string, policy Policy, holds Guard, live func(room string) bool, now time.Time, dry bool) Report {
	report := Report{At: now.Unix(), Dry: dry}
	for _, record := range all.List() {
		if !record.State.Done() || (record.Instance != "" && (live == nil || live(record.Instance))) {
			continue
		}
		days := daysSince(record, now)
		kind := due(policy, record, days)
		if kind == "" {
			continue
		}
		action := Action{Task: record.ID, Brief: record.Brief, Kind: kind, State: string(record.State), Days: days}
		target := filepath.Join(root, record.ID)
		if kind == WorkingFolder {
			target = filepath.Join(target, "workspace")
		}
		action.Bytes = size(target)
		if kind == WorkingFolder && action.Bytes == 0 {
			continue
		}
		var failed error
		held, err := holds.UnlessHeld(record, func() error {
			if !dry {
				failed = remove(all, record, kind, target)
			}
			return nil
		})
		switch {
		case err != nil:
			action.Failed = "the holds could not be read: " + err.Error()
		case held:
			report.Held = append(report.Held, record.ID)
			continue
		case failed != nil:
			action.Failed = failed.Error()
		case !dry:
			report.Freed += action.Bytes
		}
		report.Actions = append(report.Actions, action)
	}
	sort.Strings(report.Held)
	return report
}

// due is what the policy says a task has outlived, the record first because it
// takes the working folder with it.
func due(policy Policy, record tasks.Record, days int) string {
	state := string(record.State)
	if limit := policy.RecordDays[state]; limit > 0 && days >= limit {
		return Record
	}
	if limit := policy.WorkingFolderDays[state]; limit > 0 && days >= limit {
		return WorkingFolder
	}
	return ""
}

// remove takes a task's files and nothing from the usage index: its rows hold metadata and no content, and a rebuild
// only adds what the files that remain hold.
func remove(all Tasks, record tasks.Record, kind, target string) error {
	if kind == Record {
		return all.Remove(record.ID)
	}
	return os.RemoveAll(target)
}

// daysSince is whole days since the task ended, or was queued when it has no end.
func daysSince(record tasks.Record, now time.Time) int {
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

// Scanned is a read-only view of the tasks on disk, for a dry run from the
// command line while a server runs.
type Scanned struct{ Root string }

func (s Scanned) List() []tasks.Record {
	all, _ := tasks.Scan(s.Root)
	return all
}

func (Scanned) Remove(id string) error { return fmt.Errorf("a dry run removes nothing: %s", id) }
