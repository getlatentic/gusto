// Package setup is a configured agent: skills to give it and MCP servers it
// may call, named and kept so it can be started again without being
// described again.
//
// The two are declared by different people. A skill is instructions, text a
// person writes and anyone may. An MCP server is a program the box runs, or an
// address it calls, so it is the deployment's to offer: its operator decides
// what can run inside a task and what it can reach, and a setup picks from that.
package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/pkg/gateway"
	"github.com/getlatentic/gusto/pkg/job"
)

// Setup is one configuration, as kept.
type Setup struct {
	Name    string   `json:"name"`
	Servers []string `json:"servers,omitempty"`
	// Integrations are the integrations whose skills and servers the setup is given, by name, in the version an administrator has
	// turned on when a task starts.
	Integrations []string `json:"integrations,omitempty"`
	// Starters are things to say that show what the setup is for, offered in
	// an empty thread.
	Starters []string `json:"starters,omitempty"`
	// Off says an administrator made the setup and has not turned it on: it is not offered, and no task starts with it. It is the flag
	// that is kept, and not "on", so that a setup kept before there was one is on.
	Off  bool   `json:"off,omitempty"`
	Who  string `json:"who,omitempty"`
	Made int64  `json:"madeAt"`
	// Wrote is the fingerprint of each SKILL.md gusto itself last wrote, so a
	// skill still matching it can take a newer version and one a person has
	// edited is left alone.
	Wrote map[string]string `json:"wrote,omitempty"`
	// Skills is filled from the folder rather than kept in the file, so the
	// folder is the one account of what a setup holds.
	Skills []Skill `json:"-"`
	// Uses is what each integration named gives the setup now. It is worked out when the setup is read.
	Uses []Use `json:"-"`
}

// Skill is one skill: a folder holding a SKILL.md.
type Skill struct {
	Name        string
	Description string
	Body        string
}

// Store is every setup under one directory, one folder each.
type Store struct {
	root         string
	guard        sync.Mutex
	integrations Integrations
}

func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) dir(name string) string { return filepath.Join(s.root, name) }

func (s *Store) skillsDir(name string) string { return filepath.Join(s.dir(name), "skills") }

// Save writes a setup's name and servers, keeping whatever skills it has.
func (s *Store) Save(wanted Setup, offered Servers) error {
	if !job.Named(wanted.Name) {
		return fmt.Errorf("%q cannot name a setup: use lower case, digits and hyphens", wanted.Name)
	}
	for _, server := range wanted.Servers {
		if _, found := offered[server]; !found {
			return fmt.Errorf("this deployment offers no MCP server called %s", server)
		}
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	if err := s.checkIntegrations(wanted.Integrations); err != nil {
		return err
	}
	if err := os.MkdirAll(s.skillsDir(wanted.Name), 0o755); err != nil {
		return err
	}
	if wanted.Made == 0 {
		wanted.Made = time.Now().Unix()
	}
	return s.saveLocked(wanted)
}

func (s *Store) saveLocked(wanted Setup) error {
	text, err := json.MarshalIndent(wanted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir(wanted.Name), "setup.json"), text, 0o644)
}

// Get reads one setup and its skills.
func (s *Store) Get(name string) (Setup, bool) {
	if !job.Named(name) {
		return Setup{}, false
	}
	text, err := os.ReadFile(filepath.Join(s.dir(name), "setup.json"))
	if err != nil {
		return Setup{}, false
	}
	var held Setup
	if json.Unmarshal(text, &held) != nil {
		return Setup{}, false
	}
	held.Skills = s.skills(name)
	held.Uses = s.uses(held)
	return held, true
}

// List is every setup, newest first.
func (s *Store) List() []Setup {
	entries, _ := os.ReadDir(s.root)
	var all []Setup
	for _, entry := range entries {
		if held, found := s.Get(entry.Name()); found && entry.IsDir() {
			all = append(all, held)
		}
	}
	sort.Slice(all, func(a, b int) bool { return all[a].Made > all[b].Made })
	return all
}

// Write puts a skill into a setup, replacing one of the same name. The
// SKILL.md is written in the Agent Skills format every agent here reads: a
// name and a description an agent chooses by, then the instructions.
func (s *Store) Write(setup string, skill Skill) error {
	if !job.Named(skill.Name) {
		return fmt.Errorf("%q cannot name a skill: use lower case, digits and hyphens", skill.Name)
	}
	description := strings.Join(strings.Fields(skill.Description), " ")
	if description == "" {
		return errors.New("a skill needs a description: it is how an agent decides to use it")
	}
	if _, found := s.Get(setup); !found {
		return fmt.Errorf("no setup called %s", setup)
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	return s.put(setup, skill.Name, document(skill.Name, description, skill.Body))
}

func document(name, description, body string) string {
	return "---\nname: " + name + "\ndescription: " + quote(description) + "\n---\n\n" + strings.TrimSpace(body) + "\n"
}

func (s *Store) skillFile(setup, skill string) string {
	return filepath.Join(s.skillsDir(setup), skill, "SKILL.md")
}

func (s *Store) put(setup, skill, text string) error {
	if err := s.archive(setup, skill); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.skillFile(setup, skill)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.skillFile(setup, skill), []byte(text), 0o644)
}

// Drop takes a skill out of a setup.
func (s *Store) Drop(setup, skill string) error {
	if !job.Named(setup) || !job.Named(skill) {
		return fmt.Errorf("no skill called %s in %s", skill, setup)
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	if err := s.archive(setup, skill); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.skillsDir(setup), skill))
}

func (s *Store) skills(setup string) []Skill {
	entries, _ := os.ReadDir(s.skillsDir(setup))
	var held []Skill
	for _, entry := range entries {
		text, err := os.ReadFile(filepath.Join(s.skillsDir(setup), entry.Name(), "SKILL.md"))
		if err != nil || !job.Named(entry.Name()) {
			continue
		}
		skill := parse(string(text))
		skill.Name = entry.Name()
		held = append(held, skill)
	}
	return held
}

// Carry copies what a task needs of a setup into the task's own directory: the
// skills, and the definitions of the servers it names. It returns the names
// for job.json, which is evidence and so holds no definition.
func (s *Store) Carry(name string, offered Servers, dir job.Dir) (*job.Setup, error) {
	return s.CarryFor(name, offered, dir, "")
}

// CarryFor is Carry for the person who asked. An integration that is on for some people only is not carried for anybody else, and the
// task does not start, since a task that quietly lacked what its setup names would run as a bare agent under the setup's name.
func (s *Store) CarryFor(name string, offered Servers, dir job.Dir, asker string) (*job.Setup, error) {
	held, found := s.Get(name)
	if !found {
		return nil, fmt.Errorf("no setup called %s", name)
	}
	if held.Off {
		return nil, fmt.Errorf("the setup %s is off: an administrator turns it on", name)
	}
	carried := &job.Setup{Name: held.Name, Servers: held.Servers}
	if err := os.MkdirAll(dir.SetupDir(), 0o700); err != nil {
		return nil, err
	}
	for _, skill := range held.Skills {
		carried.Skills = append(carried.Skills, skill.Name)
	}
	if len(held.Skills) > 0 {
		if err := os.CopyFS(filepath.Join(dir.SetupDir(), "skills"), os.DirFS(s.skillsDir(name))); err != nil {
			return nil, err
		}
	}
	servers := make([]job.Server, 0, len(held.Servers))
	for _, server := range held.Servers {
		definition, found := offered[server]
		if !found {
			return nil, fmt.Errorf("the setup %s names %s, which this deployment no longer offers", name, server)
		}
		servers = append(servers, definition)
	}
	var behind []gateway.Server
	if err := s.carryIntegrations(held, dir, carried, &behind, asker); err != nil {
		return nil, err
	}
	if err := writeSecret(filepath.Join(dir.SetupDir(), "servers.json"), servers); err != nil {
		return nil, err
	}
	return carried, writeSecret(filepath.Join(dir.SetupDir(), GatewayFile), behind)
}

// GatewayFile is where the definitions of the servers behind the gateway wait in a task's setup directory. The proxy reads it and
// the sandbox cannot: it is the owner's alone.
const GatewayFile = "gateway.json"

// writeSecret writes definitions, which may hold a credential, for their owner alone. Nothing to write is no file.
func writeSecret[T any](path string, definitions []T) error {
	if len(definitions) == 0 {
		return nil
	}
	text, err := json.Marshal(definitions)
	if err != nil {
		return err
	}
	return os.WriteFile(path, text, 0o600)
}

// Fits says why an agent cannot take a setup, or nothing when it can.
func (held Setup) Fits(skills, mcp bool) string {
	given := 0
	served := len(held.Servers)
	for _, use := range held.Uses {
		given += use.Skills
		served += use.Servers
	}
	switch {
	case (len(held.Skills) > 0 || given > 0) && !skills:
		return "it takes no skills"
	case served > 0 && !mcp:
		return "it takes no MCP servers"
	}
	return ""
}
