// Package retention removes finished work on a schedule an operator sets.
//
// A finished job keeps two things for different lengths of time: its workspace,
// which is the bulk of the disk and what shipping and replay need, and its
// record, which is the signed evidence and is small. The workspace goes first.
// A job on hold, or asked for by a person on hold, is kept whole.
package retention

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/jobs"
)

// PolicyFile is where a retention directory keeps its policy.
const PolicyFile = "policy.json"

// States a job can finish in, and so the states a policy is set for.
var States = []string{string(jobs.Passed), string(jobs.Failed), string(jobs.Interrupted)}

// Policy is how many days after a job ends it loses its workspace and then its
// record, by the state it ended in. A state with no entry, or zero, is kept.
type Policy struct {
	WorkspaceDays map[string]int `json:"workspaceDays,omitempty"`
	RecordDays    map[string]int `json:"recordDays,omitempty"`
}

// Any says whether the policy removes anything at all.
func (p Policy) Any() bool {
	for _, days := range p.WorkspaceDays {
		if days > 0 {
			return true
		}
	}
	for _, days := range p.RecordDays {
		if days > 0 {
			return true
		}
	}
	return false
}

// Check refuses a policy that names a state a job cannot end in, a negative
// number, or a record that would go before its workspace.
func (p Policy) Check() error {
	for name, set := range map[string]map[string]int{"workspaceDays": p.WorkspaceDays, "recordDays": p.RecordDays} {
		for state, days := range set {
			if !contains(States, state) {
				return fmt.Errorf("%s names %q, which is not a state a job ends in: %s", name, state, strings.Join(States, ", "))
			}
			if days < 0 {
				return fmt.Errorf("%s for %s is %d: days cannot be negative", name, state, days)
			}
		}
	}
	for state, records := range p.RecordDays {
		if workspace := p.WorkspaceDays[state]; records > 0 && workspace > records {
			return fmt.Errorf("a %s job's workspace is kept %d days and its record %d: the record cannot go first", state, workspace, records)
		}
	}
	return nil
}

func contains(all []string, one string) bool {
	for _, each := range all {
		if each == one {
			return true
		}
	}
	return false
}

// LoadPolicy reads the policy in dir. A directory with none keeps everything.
func LoadPolicy(dir string) (Policy, error) {
	text, err := os.ReadFile(filepath.Join(dir, PolicyFile))
	if errors.Is(err, os.ErrNotExist) {
		return Policy{}, nil
	}
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	decoder := json.NewDecoder(strings.NewReader(string(text)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", filepath.Join(dir, PolicyFile), err)
	}
	if err := p.Check(); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", filepath.Join(dir, PolicyFile), err)
	}
	return p, nil
}

// Rows is the policy a state at a time, for a page to show.
func (p Policy) Rows() []Row {
	var rows []Row
	for _, state := range States {
		rows = append(rows, Row{State: state, Workspace: p.WorkspaceDays[state], Record: p.RecordDays[state]})
	}
	return rows
}

// Row is one state's retention. Zero is kept for ever.
type Row struct {
	State             string
	Workspace, Record int
}
