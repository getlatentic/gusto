// Package setup is a configured harness: skills to give it and MCP servers it
// may call, named and kept so it can be started again without being
// described again.
//
// The two are declared by different people. A skill is instructions, text a
// person writes and anyone may. An MCP server is a program the box runs, or an
// address it calls, so it is the deployment's to offer: its operator decides
// what can run inside a job and what it can reach, and a setup picks from that.
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

	"github.com/getlatentic/haas/internal/job"
)

// Setup is one configuration, as kept.
type Setup struct {
	Name    string   `json:"name"`
	Servers []string `json:"servers,omitempty"`
	// Starters are things to say that show what the setup is for, offered in
	// an empty thread.
	Starters []string `json:"starters,omitempty"`
	Who      string   `json:"who,omitempty"`
	Made     int64    `json:"madeAt"`
	// Skills is filled from the folder rather than kept in the file, so the
	// folder is the one account of what a setup holds.
	Skills []Skill `json:"-"`
}

// Skill is one skill: a folder holding a SKILL.md.
type Skill struct {
	Name        string
	Description string
	Body        string
}

// Store is every setup under one directory, one folder each.
type Store struct {
	root  string
	guard sync.Mutex
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
	if err := os.MkdirAll(s.skillsDir(wanted.Name), 0o755); err != nil {
		return err
	}
	if wanted.Made == 0 {
		wanted.Made = time.Now().Unix()
	}
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
// SKILL.md is written in the Agent Skills format every harness here reads: a
// name and a description a harness chooses by, then the instructions.
func (s *Store) Write(setup string, skill Skill) error {
	if !job.Named(skill.Name) {
		return fmt.Errorf("%q cannot name a skill: use lower case, digits and hyphens", skill.Name)
	}
	description := strings.Join(strings.Fields(skill.Description), " ")
	if description == "" {
		return errors.New("a skill needs a description: it is how a harness decides to use it")
	}
	if _, found := s.Get(setup); !found {
		return fmt.Errorf("no setup called %s", setup)
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	dir := filepath.Join(s.skillsDir(setup), skill.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	document := "---\nname: " + skill.Name + "\ndescription: " + quote(description) + "\n---\n\n" + strings.TrimSpace(skill.Body) + "\n"
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(document), 0o644)
}

// Drop takes a skill out of a setup.
func (s *Store) Drop(setup, skill string) error {
	if !job.Named(setup) || !job.Named(skill) {
		return fmt.Errorf("no skill called %s in %s", skill, setup)
	}
	s.guard.Lock()
	defer s.guard.Unlock()
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

// Carry copies what a job needs of a setup into the job's own directory: the
// skills, and the definitions of the servers it names. It returns the names
// for job.json, which is evidence and so holds no definition.
func (s *Store) Carry(name string, offered Servers, dir job.Dir) (*job.Setup, error) {
	held, found := s.Get(name)
	if !found {
		return nil, fmt.Errorf("no setup called %s", name)
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
	if len(servers) == 0 {
		return carried, nil
	}
	text, err := json.Marshal(servers)
	if err != nil {
		return nil, err
	}
	return carried, os.WriteFile(filepath.Join(dir.SetupDir(), "servers.json"), text, 0o600)
}

// Fits says why a harness cannot take a setup, or nothing when it can.
func (held Setup) Fits(skills, mcp bool) string {
	switch {
	case len(held.Skills) > 0 && !skills:
		return "it takes no skills"
	case len(held.Servers) > 0 && !mcp:
		return "it takes no MCP servers"
	}
	return ""
}
