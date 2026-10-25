// Package crew keeps a workspace's agents: each a member of the workspace with a name people mention it by, the agent type and
// model it runs with, the skills it is given, and who may work with it. An agent is made once and added to as many rooms as it
// is wanted in, so a change to it reaches every room it is in.
package crew

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Agent is one of the workspace's agents.
type Agent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Specialty is what it is best at, in a few words, so teammates know what to ask it.
	Specialty string `json:"specialty,omitempty"`
	Harness   string `json:"harness"`
	Model     string `json:"model,omitempty"`
	// Setup is the skill set it is given, by name, or nothing for none.
	Setup string `json:"setup,omitempty"`
	// Persona is how it works and talks, which it is told at the start of every turn.
	Persona string `json:"persona,omitempty"`
	// Owner made it and decides who else may work with it; Access is everybody else who may.
	Owner   string   `json:"owner"`
	Access  []string `json:"access,omitempty"`
	Created int64    `json:"createdAt"`
}

// named is what an agent can be called: a letter first, then letters, digits, spaces and a few marks, as people write names.
var named = regexp.MustCompile(`^[\p{L}][\p{L}\p{N} '’._-]{0,31}$`)

// ErrNotFound is an agent that is not there.
var ErrNotFound = errors.New("no such agent")

// Store is every agent, a file each, under one directory.
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

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".json") }

// List is every agent, by name.
func (s *Store) List() []Agent {
	entries, _ := os.ReadDir(s.dir)
	var all []Agent
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if held, err := s.read(strings.TrimSuffix(name, ".json")); err == nil {
			all = append(all, held)
		}
	}
	sort.Slice(all, func(i, j int) bool { return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name) })
	return all
}

func (s *Store) read(id string) (Agent, error) {
	text, err := os.ReadFile(s.path(id))
	if err != nil {
		return Agent{}, ErrNotFound
	}
	var held Agent
	if err := json.Unmarshal(text, &held); err != nil {
		return Agent{}, fmt.Errorf("agent %s cannot be read: %w", id, err)
	}
	return held, nil
}

// Get is one agent.
func (s *Store) Get(id string) (Agent, bool) {
	if !validID(id) {
		return Agent{}, false
	}
	held, err := s.read(id)
	return held, err == nil
}

// Make keeps a new agent for its owner. Its name must be one no other agent has, since people mention agents by name.
func (s *Store) Make(wanted Agent) (Agent, error) {
	s.guard.Lock()
	defer s.guard.Unlock()
	wanted.Owner = strings.ToLower(strings.TrimSpace(wanted.Owner))
	if wanted.Owner == "" {
		return Agent{}, errors.New("an agent belongs to somebody")
	}
	if wanted.Harness == "" {
		return Agent{}, errors.New("choose how the agent runs")
	}
	if err := s.nameFree(wanted.Name, ""); err != nil {
		return Agent{}, err
	}
	wanted.Name = strings.TrimSpace(wanted.Name)
	wanted.ID = newID()
	wanted.Created = time.Now().Unix()
	wanted.Access = nil
	return wanted, s.write(wanted)
}

// Change changes an agent and keeps it. The change cannot move its id, its owner or when it was made.
func (s *Store) Change(id string, change func(*Agent) error) (Agent, error) {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, ok := s.Get(id)
	if !ok {
		return Agent{}, ErrNotFound
	}
	changed := held
	if err := change(&changed); err != nil {
		return Agent{}, err
	}
	changed.ID, changed.Owner, changed.Created = held.ID, held.Owner, held.Created
	changed.Name = strings.TrimSpace(changed.Name)
	if err := s.nameFree(changed.Name, id); err != nil {
		return Agent{}, err
	}
	changed.Access = people(changed.Access, changed.Owner)
	return changed, s.write(changed)
}

// Remove deletes an agent.
func (s *Store) Remove(id string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	if !validID(id) {
		return ErrNotFound
	}
	if err := os.Remove(s.path(id)); err != nil {
		return ErrNotFound
	}
	return nil
}

// MayUse says whether a person may work with an agent: put it in a room, or talk to it.
func (held Agent) MayUse(who string) bool {
	who = strings.ToLower(strings.TrimSpace(who))
	return who != "" && (who == held.Owner || slices.Contains(held.Access, who))
}

func (s *Store) nameFree(name, besides string) error {
	name = strings.TrimSpace(name)
	if !named.MatchString(name) {
		return fmt.Errorf("%q cannot name an agent: start with a letter, and use up to 32 letters, digits and spaces", name)
	}
	for _, other := range s.List() {
		if other.ID != besides && strings.EqualFold(other.Name, name) {
			return fmt.Errorf("an agent is already called %s", other.Name)
		}
	}
	return nil
}

// people is a list of addresses tidied: lower case, each once, without the owner, who always has access.
func people(listed []string, owner string) []string {
	var kept []string
	for _, who := range listed {
		who = strings.ToLower(strings.TrimSpace(who))
		if who != "" && who != owner && !slices.Contains(kept, who) {
			kept = append(kept, who)
		}
	}
	return kept
}

func (s *Store) write(held Agent) error {
	text, err := json.MarshalIndent(held, "", "  ")
	if err != nil {
		return err
	}
	scratch, err := os.CreateTemp(s.dir, ".agent-*")
	if err != nil {
		return err
	}
	defer os.Remove(scratch.Name())
	if _, err := scratch.Write(text); err != nil {
		scratch.Close()
		return err
	}
	if err := scratch.Close(); err != nil {
		return err
	}
	return os.Rename(scratch.Name(), s.path(held.ID))
}

func newID() string {
	raw := make([]byte, 6)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func validID(id string) bool {
	if len(id) != 12 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
