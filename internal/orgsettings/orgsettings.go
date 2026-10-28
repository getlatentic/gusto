// Package orgsettings is what an organization's admins choose for it, kept in one file
// in its data directory. Only the organization's own server writes it, so there is one
// writer, and it reads the file as it goes.
package orgsettings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// Settings are the choices there are.
type Settings struct {
	// MonthlyBudgetUSD is what the organization means to spend in a calendar month, UTC.
	// Zero is no budget.
	MonthlyBudgetUSD float64 `json:"monthlyBudgetUsd,omitempty"`
	// DefaultHarness and DefaultModel are what a new chat runs on until a person chooses their own. Empty is the first that can run.
	DefaultHarness string `json:"defaultHarness,omitempty"`
	DefaultModel   string `json:"defaultModel,omitempty"`
	// PlainRunTokens gives an agent in a sandbox a token for its run in place of one bound to a key only its sandbox holds.
	// Unset is bound.
	PlainRunTokens bool `json:"plainRunTokens,omitempty"`
}

// File is the organization's settings kept in a file.
type File struct {
	path string
	mu   sync.Mutex
}

// Open is the settings at path. A file that is not there is settings that choose
// nothing, since an organization starts with none.
func Open(path string) *File { return &File{path: path} }

// Read is the settings now. A file that is there and cannot be read is an error and not
// settings that choose nothing: a page that showed no budget would be wrong, and would
// look right.
func (f *File) Read() (Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read()
}

func (f *File) read() (Settings, error) {
	text, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("the settings in %s cannot be read: %w", f.path, err)
	}
	var settings Settings
	if err := json.Unmarshal(text, &settings); err != nil {
		return Settings{}, fmt.Errorf("the settings in %s are not valid: %w", f.path, err)
	}
	if settings.MonthlyBudgetUSD < 0 {
		return Settings{}, fmt.Errorf("the settings in %s name a budget below nothing", f.path)
	}
	return settings, nil
}

// Change makes one change to the settings and keeps the result. It refuses to change
// settings it cannot read, so a damaged file is not written over.
func (f *File) Change(change func(*Settings)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	settings, err := f.read()
	if err != nil {
		return err
	}
	change(&settings)
	return atomicfile.Write(f.path, settings, 0o600)
}
