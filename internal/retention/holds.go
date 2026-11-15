package retention

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
	"github.com/getlatentic/gusto/internal/tasks"
)

// HoldsFile is where a retention directory keeps its holds.
const HoldsFile = "holds.json"

// Hold keeps work from being removed, and says who asked and why.
type Hold struct {
	By  string `json:"by"`
	At  int64  `json:"at"`
	Why string `json:"why,omitempty"`
}

// Held is every hold in force: on single tasks, and on everything a person asked for.
type Held struct {
	Tasks  map[string]Hold `json:"jobs,omitempty"`
	People map[string]Hold `json:"people,omitempty"`
}

// Covers says whether a task is held, by itself or by who asked for it.
func (h Held) Covers(record tasks.Record) (Hold, bool) {
	if hold, held := h.Tasks[record.ID]; held {
		return hold, true
	}
	if record.Asked != "" {
		if hold, held := h.People[strings.ToLower(record.Asked)]; held {
			return hold, true
		}
	}
	return Hold{}, false
}

// Guard says whether a task is held, and removes it when it is not, as one step: a hold placed while a sweep runs keeps what the
// sweep has not yet removed.
type Guard interface {
	UnlessHeld(record tasks.Record, remove func() error) (held bool, err error)
}

// UnlessHeld is the holds as they were read, for a sweep that plans and removes nothing.
func (h Held) UnlessHeld(record tasks.Record, remove func() error) (bool, error) {
	if _, held := h.Covers(record); held {
		return true, nil
	}
	return false, remove()
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

// HoldTask keeps one task whole until it is released.
func (h *Holds) HoldTask(id, by, why string) error {
	return h.change(func(held *Held) {
		if held.Tasks == nil {
			held.Tasks = map[string]Hold{}
		}
		held.Tasks[id] = Hold{By: by, At: time.Now().Unix(), Why: why}
	})
}

// ReleaseTask lets a task be removed on the policy's schedule again.
func (h *Holds) ReleaseTask(id string) error {
	return h.change(func(held *Held) { delete(held.Tasks, id) })
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

// UnlessHeld reads the holds and removes the task when none covers it, all while no hold can be placed.
func (h *Holds) UnlessHeld(record tasks.Record, remove func() error) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	held, err := h.read()
	if err != nil {
		return false, err
	}
	if _, covered := held.Covers(record); covered {
		return true, nil
	}
	return false, remove()
}

// ForgetGone ends the holds on tasks that are no longer kept, which nothing else would end.
func (h *Holds) ForgetGone(kept func(id string) bool) error {
	held, err := h.Read()
	if err != nil {
		return err
	}
	var gone []string
	for id := range held.Tasks {
		if !kept(id) {
			gone = append(gone, id)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	return h.change(func(held *Held) {
		for _, id := range gone {
			delete(held.Tasks, id)
		}
	})
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
	return atomicfile.WriteFile(h.path, append(text, '\n'), 0o640)
}
