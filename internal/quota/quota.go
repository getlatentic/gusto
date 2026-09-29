// Package quota is what an organisation's server may run, kept in a file its
// front door writes and the server reads as it goes. A hosted organisation's
// limit changes when its admin confirms an address, and the server that is
// running must see that without a restart.
package quota

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Quota is what may be run. Zero is none.
type Quota struct {
	// Jobs is how many jobs may be started: in all, or in a calendar month when
	// Monthly is set.
	Jobs    int  `json:"jobs"`
	Monthly bool `json:"monthly,omitempty"`
	// Concurrent is how many jobs may run at once.
	Concurrent int `json:"concurrent"`
	// Invites says whether colleagues may be invited, Members how many people
	// there may be in all, and Computers how many computers.
	Invites   bool `json:"invites"`
	Members   int  `json:"members"`
	Computers int  `json:"computers"`
	// Note is a sentence added to a refusal: how the limit is lifted.
	Note string `json:"note,omitempty"`
}

// ErrExceeded is what a refusal is: it wraps to this, so a caller can answer
// "too many" and not "broken".
var ErrExceeded = errors.New("this organisation has reached its limit")

// Refuse is nil while started jobs are fewer than the quota allows, and a
// sentence a person can act on when they are not.
func (q Quota) Refuse(started int) error {
	if started < q.Jobs {
		return nil
	}
	within := "in all"
	if q.Monthly {
		within = "a month"
	}
	jobs := "jobs"
	if q.Jobs == 1 {
		jobs = "job"
	}
	message := fmt.Sprintf("This organisation may start %d %s %s and has started %d.", q.Jobs, jobs, within, started)
	if q.Note != "" {
		message += " " + q.Note
	}
	return exceeded(message)
}

// exceeded is a refusal: its text is the sentence, and it matches ErrExceeded.
type exceeded string

func (e exceeded) Error() string        { return string(e) }
func (e exceeded) Is(target error) bool { return target == ErrExceeded }

// Write puts a quota where a server reads it, by renaming a finished file into
// place so a server never reads half of one.
func Write(path string, q Quota) error {
	text, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".quota-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(text, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

// File is a quota kept in a file. It is read again when a second has passed, so
// a limit that was lifted takes effect at once and a busy queue does not read a
// file for every job it looks at.
type File struct {
	path string
	mu   sync.Mutex
	at   time.Time
	last Quota
	err  error
	now  func() time.Time
}

// Open is the quota at path. Nothing is read until it is asked for.
func Open(path string) *File { return &File{path: path, now: time.Now} }

// Current is the quota now. A file that cannot be read is an error and not a
// quota of none or of any: a server that was given limits and lost them starts
// nothing rather than everything.
func (f *File) Current() (Quota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if now := f.now(); f.at.IsZero() || now.Sub(f.at) >= time.Second {
		f.at = now
		f.last, f.err = read(f.path)
	}
	return f.last, f.err
}

func read(path string) (Quota, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return Quota{}, fmt.Errorf("the limits in %s cannot be read: %w", path, err)
	}
	var q Quota
	if err := json.Unmarshal(text, &q); err != nil {
		return Quota{}, fmt.Errorf("the limits in %s are not valid: %w", path, err)
	}
	if q.Jobs < 0 || q.Concurrent < 0 {
		return Quota{}, fmt.Errorf("the limits in %s are negative", path)
	}
	return q, nil
}

// RefuseComputers is nil while the computers there are number fewer than the quota
// allows, and a sentence a person can act on when they do not.
func (q Quota) RefuseComputers(have int) error {
	if have < q.Computers {
		return nil
	}
	computers := "computers"
	if q.Computers == 1 {
		computers = "computer"
	}
	message := fmt.Sprintf("This organisation may have %d %s and has %d.", q.Computers, computers, have)
	if q.Note != "" {
		message += " " + q.Note
	}
	return exceeded(message)
}
