// Package preset keeps what a person saves to start the same kind of chat again: a name, an engine, a model and a role.
package preset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/getlatentic/haas/internal/atomicfile"
)

// Limits on what is kept for one person.
const (
	MaxPresets = 50
	MaxName    = 60
)

// Preset is a saved way to start a chat.
type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	// Role names a ready-made kind of work whose skills the chat is given. It is empty for a chat with none.
	Role  string `json:"role,omitempty"`
	Saved int64  `json:"savedAt"`
}

// Store is every person's presets under one directory, a file each.
type Store struct {
	dir   string
	guard sync.Mutex
}

// Open is the store in dir, made if it is not there.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

type file struct {
	Who     string   `json:"who"`
	Presets []Preset `json:"presets"`
}

func (s *Store) path(who string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(who)))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:8])+".json")
}

func (s *Store) read(who string) (file, error) {
	text, err := os.ReadFile(s.path(who))
	if errors.Is(err, os.ErrNotExist) {
		return file{Who: who}, nil
	}
	if err != nil {
		return file{}, err
	}
	var held file
	if err := json.Unmarshal(text, &held); err != nil {
		return file{}, fmt.Errorf("the presets of %s cannot be read: %w", who, err)
	}
	return held, nil
}

// List is a person's presets, the newest first.
func (s *Store) List(who string) []Preset {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, err := s.read(who)
	if err != nil {
		return nil
	}
	listed := make([]Preset, len(held.Presets))
	for i, each := range held.Presets {
		listed[len(held.Presets)-1-i] = each
	}
	return listed
}

// Get is one of a person's presets.
func (s *Store) Get(who, id string) (Preset, bool) {
	for _, each := range s.List(who) {
		if each.ID == id {
			return each, true
		}
	}
	return Preset{}, false
}

// Save keeps a preset for a person, under a name made unique among theirs.
func (s *Store) Save(who string, wanted Preset) (Preset, error) {
	wanted.Name = strings.TrimSpace(wanted.Name)
	switch {
	case who == "":
		return Preset{}, errors.New("a preset belongs to somebody")
	case wanted.Name == "":
		return Preset{}, errors.New("give the preset a name")
	case utf8.RuneCountInString(wanted.Name) > MaxName:
		return Preset{}, fmt.Errorf("a preset's name may be at most %d characters", MaxName)
	case wanted.Harness == "":
		return Preset{}, errors.New("a preset needs an engine")
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	held, err := s.read(who)
	if err != nil {
		return Preset{}, err
	}
	if len(held.Presets) >= MaxPresets {
		return Preset{}, fmt.Errorf("you have %d presets, which is the most kept; remove one first", MaxPresets)
	}
	for _, each := range held.Presets {
		if strings.EqualFold(each.Name, wanted.Name) {
			return Preset{}, fmt.Errorf("you already have a preset called %q", each.Name)
		}
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", who, wanted.Name, time.Now().UnixNano())))
	wanted.ID = hex.EncodeToString(sum[:5])
	wanted.Saved = time.Now().Unix()
	held.Who = who
	held.Presets = append(held.Presets, wanted)
	return wanted, atomicfile.Write(s.path(who), held, 0o640)
}

// Delete removes one of a person's presets, and says whether there was one.
func (s *Store) Delete(who, id string) (bool, error) {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, err := s.read(who)
	if err != nil {
		return false, err
	}
	kept := held.Presets[:0:0]
	for _, each := range held.Presets {
		if each.ID != id {
			kept = append(kept, each)
		}
	}
	if len(kept) == len(held.Presets) {
		return false, nil
	}
	held.Presets = kept
	return true, atomicfile.Write(s.path(who), held, 0o640)
}
