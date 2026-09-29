package retention

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/jobs"
)

// HoldsFile is where a retention directory keeps its holds.
const HoldsFile = "holds.json"

// Hold keeps work from being removed, and says who asked and why.
type Hold struct {
	By  string `json:"by"`
	At  int64  `json:"at"`
	Why string `json:"why,omitempty"`
}

// Held is every hold in force: on single jobs, and on everything a person asked for.
type Held struct {
	Jobs   map[string]Hold `json:"jobs,omitempty"`
	People map[string]Hold `json:"people,omitempty"`
}

// Covers says whether a job is held, by itself or by who asked for it.
func (h Held) Covers(record jobs.Record) (Hold, bool) {
	if hold, held := h.Jobs[record.ID]; held {
		return hold, true
	}
	if record.Asked != "" {
		if hold, held := h.People[strings.ToLower(record.Asked)]; held {
			return hold, true
		}
	}
	return Hold{}, false
}

// Holds is the file the holds are kept in. Every read goes to the file, so a
// sweep in another process sees a hold made a moment ago.
type Holds struct {
	path string
	mu   sync.Mutex
}

// OpenHolds keeps holds in dir.
func OpenHolds(dir string) *Holds { return &Holds{path: filepath.Join(dir, HoldsFile)} }

// Read is every hold in force.
func (h *Holds) Read() (Held, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.read()
}

func (h *Holds) read() (Held, error) {
	text, err := os.ReadFile(h.path)
	if errors.Is(err, os.ErrNotExist) {
		return Held{}, nil
	}
	if err != nil {
		return Held{}, err
	}
	var held Held
	return held, json.Unmarshal(text, &held)
}

// HoldJob keeps one job whole until it is released.
func (h *Holds) HoldJob(id, by, why string) error {
	return h.change(func(held *Held) {
		if held.Jobs == nil {
			held.Jobs = map[string]Hold{}
		}
		held.Jobs[id] = Hold{By: by, At: time.Now().Unix(), Why: why}
	})
}

// ReleaseJob lets a job be removed on the policy's schedule again.
func (h *Holds) ReleaseJob(id string) error {
	return h.change(func(held *Held) { delete(held.Jobs, id) })
}

// HoldPerson keeps everything a person asked for, now and later, whole.
func (h *Holds) HoldPerson(who, by, why string) error {
	return h.change(func(held *Held) {
		if held.People == nil {
			held.People = map[string]Hold{}
		}
		held.People[strings.ToLower(who)] = Hold{By: by, At: time.Now().Unix(), Why: why}
	})
}

// ReleasePerson ends a hold on a person.
func (h *Holds) ReleasePerson(who string) error {
	return h.change(func(held *Held) { delete(held.People, strings.ToLower(who)) })
}

func (h *Holds) change(edit func(*Held)) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	held, err := h.read()
	if err != nil {
		return err
	}
	edit(&held)
	text, err := json.MarshalIndent(held, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o750); err != nil {
		return err
	}
	scratch := h.path + ".tmp"
	if err := os.WriteFile(scratch, append(text, '\n'), 0o640); err != nil {
		return err
	}
	return os.Rename(scratch, h.path)
}
