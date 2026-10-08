package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/job"
)

// mostEarlier is how many earlier versions of one skill are kept to go back to.
const mostEarlier = 10

func (s *Store) earlierDir(setup, skill string) string {
	return filepath.Join(s.dir(setup), "earlier", skill)
}

// archive keeps the skill as it is, before it is replaced or dropped, when there is one and it differs from the last kept.
func (s *Store) archive(setup, skill string) error {
	text, err := os.ReadFile(s.skillFile(setup, skill))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := s.earlierDir(setup, skill)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	kept := s.earlierNames(dir)
	if len(kept) > 0 {
		if last, err := os.ReadFile(filepath.Join(dir, kept[len(kept)-1])); err == nil && string(last) == string(text) {
			return nil
		}
	}
	stamp := time.Now().UnixNano()
	if len(kept) > 0 {
		if previous, _ := strconv.ParseInt(strings.TrimSuffix(kept[len(kept)-1], ".md"), 10, 64); stamp <= previous {
			stamp = previous + 1
		}
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.md", stamp)), text, 0o644); err != nil {
		return err
	}
	for len(kept)+1 > mostEarlier {
		os.Remove(filepath.Join(dir, kept[0]))
		kept = kept[1:]
	}
	return nil
}

func (s *Store) earlierNames(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Slice(names, func(a, b int) bool {
		left, _ := strconv.ParseInt(strings.TrimSuffix(names[a], ".md"), 10, 64)
		right, _ := strconv.ParseInt(strings.TrimSuffix(names[b], ".md"), 10, 64)
		return left < right
	})
	return names
}

// Earlier is one kept version of a skill.
type Earlier struct {
	Skill string
	// At names the version, and is when it was replaced, in nanoseconds.
	At          int64
	Fingerprint string
	// Present says the skill is in the setup now, and not dropped.
	Present bool
}

// Earlier lists the kept versions of every skill of a setup, the latest first, including skills that were dropped.
func (s *Store) Earlier(setup string) []Earlier {
	if !job.Named(setup) {
		return nil
	}
	skills, _ := os.ReadDir(filepath.Join(s.dir(setup), "earlier"))
	var all []Earlier
	for _, entry := range skills {
		if !entry.IsDir() || !job.Named(entry.Name()) {
			continue
		}
		_, there := os.Stat(s.skillFile(setup, entry.Name()))
		for _, name := range s.earlierNames(filepath.Join(s.dir(setup), "earlier", entry.Name())) {
			at, _ := strconv.ParseInt(strings.TrimSuffix(name, ".md"), 10, 64)
			text, _ := os.ReadFile(filepath.Join(s.dir(setup), "earlier", entry.Name(), name))
			all = append(all, Earlier{Skill: entry.Name(), At: at, Fingerprint: fingerprint(string(text))[:12], Present: there == nil})
		}
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].At > all[b].At })
	return all
}

// Fingerprint says what a skill is now, in a few characters, or "none" for a skill that is not there. It is what the change record
// holds of a skill, so that it says a skill changed and from what to what, and not the text.
func (s *Store) Fingerprint(setup, skill string) string {
	text, err := os.ReadFile(s.skillFile(setup, skill))
	if err != nil {
		return "none"
	}
	return fingerprint(string(text))[:12]
}

// RestoreSkill puts a kept version of a skill back, as the skill, keeping what it replaces.
func (s *Store) RestoreSkill(setup, skill string, at int64) error {
	if !job.Named(setup) || !job.Named(skill) {
		return fmt.Errorf("no skill called %s in %s", skill, setup)
	}
	text, err := os.ReadFile(filepath.Join(s.earlierDir(setup, skill), fmt.Sprintf("%d.md", at)))
	if err != nil {
		return fmt.Errorf("there is no kept version of %s from then", skill)
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	return s.put(setup, skill, string(text))
}

// SetOff turns a setup off or on. Turning one on is what makes what an administrator made available to people.
func (s *Store) SetOff(name string, off bool) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.Get(name)
	if !found {
		return fmt.Errorf("no setup called %s", name)
	}
	held.Off = off
	return s.saveLocked(held)
}
