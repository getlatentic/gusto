package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// MaxBio is how many characters a person's bio may hold. Agents in a person's rooms read it, so it is a few sentences.
const MaxBio = 600

// biosFile is where the bios are kept, beside the instances. Its name starts with a dot, so nothing takes it for an instance.
const biosFile = ".bios.json"

// bios is what each person wrote of themselves, by address in lower case.
type bios struct {
	// mu guards held. It is held across the write, so two people saving at once do not write over each other's change.
	mu   sync.Mutex
	held map[string]string
}

func readBios(root string) map[string]string {
	path := filepath.Join(root, biosFile)
	text, err := os.ReadFile(path)
	held := map[string]string{}
	if errors.Is(err, fs.ErrNotExist) {
		return held
	}
	if err == nil {
		if err = json.Unmarshal(text, &held); err == nil {
			return held
		}
	}
	kept := fmt.Sprintf("%s.damaged-%d", path, time.Now().Unix())
	if moved := os.Rename(path, kept); moved != nil {
		slog.Warn("the bios could not be read or set aside, so they start again empty and the next save writes over them", "file", path, "error", err)
	} else {
		slog.Warn("the bios file was damaged: it is kept beside its place and the bios start again empty", "file", path, "kept", kept)
	}
	return map[string]string{}
}

// Bio is what a person wrote of themselves, or nothing.
func (s *Store) Bio(who string) string {
	s.bios.mu.Lock()
	defer s.bios.mu.Unlock()
	return s.bios.held[strings.ToLower(strings.TrimSpace(who))]
}

// SetBio keeps what a person says of themselves. Empty clears it. It is written before it is held, so a write that fails changes
// nothing.
func (s *Store) SetBio(who, text string) error {
	who = strings.ToLower(strings.TrimSpace(who))
	if who == "" {
		return fmt.Errorf("a bio is a person's")
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if n := utf8.RuneCountInString(text); n > MaxBio {
		return fmt.Errorf("a bio holds at most %d characters, and this is %d", MaxBio, n)
	}
	s.bios.mu.Lock()
	defer s.bios.mu.Unlock()
	next := maps.Clone(s.bios.held)
	if text == "" {
		delete(next, who)
	} else {
		next[who] = text
	}
	if err := writeJSON(filepath.Join(s.root, biosFile), next); err != nil {
		return err
	}
	s.bios.held = next
	return nil
}
