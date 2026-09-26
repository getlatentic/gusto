// Package instance is a harness with a computer.
//
// An instance is what a person picks from the catalog and keeps: a harness, the
// model it thinks with, and a workspace that stays. A thread is one conversation
// with it, and an instance has as many as somebody wants to hold at once.
//
// The two are separate because they have different lifetimes. The workspace and
// the harness's own data directory belong to the instance and outlive every
// conversation; what the harness remembers of a conversation belongs to the
// thread and is asked for again by name when that thread is resumed.
//
// Nothing here is a second way to run things. A turn is an ordinary job that
// mounts the instance's workspace instead of cloning a repository, and it is
// queued, sandboxed, streamed and recorded exactly as any other job is.
package instance

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Instance is the computer: what a harness runs under and works in.
type Instance struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Who     string `json:"who,omitempty"`
	Started int64  `json:"startedAt"`
}

// Store is every instance under one directory.
type Store struct {
	root    string
	guard   sync.Mutex
	held    map[string]Instance
	threads map[string]map[string]Thread
}

func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	store := &Store{
		root:    root,
		held:    map[string]Instance{},
		threads: map[string]map[string]Thread{},
	}
	listing, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range listing {
		if !entry.IsDir() {
			continue
		}
		if err := store.load(filepath.Join(root, entry.Name())); err != nil {
			continue
		}
	}
	return store, nil
}

func (s *Store) Root() string { return s.root }

// Dir is where an instance keeps everything that stays.
func (s *Store) Dir(id string) string { return filepath.Join(s.root, id) }

// Workspace is the directory a harness works in, across every thread.
func (s *Store) Workspace(id string) string { return filepath.Join(s.Dir(id), "workspace") }

// State is where a harness keeps its own memory. It is outside the workspace
// because that is where harnesses put it: opencode keeps its sessions under its
// data directory, not beside the files it edits. Threads share it, and the
// harness tells them apart by the name each thread remembers.
func (s *Store) State(id string) string { return filepath.Join(s.Dir(id), "state") }

func (s *Store) threadsDir(id string) string { return filepath.Join(s.Dir(id), "threads") }

// Start makes an instance, and fills its workspace from a repository when one
// is given. Cloning happens here rather than in a turn, so it happens once and
// so a repository credential stays outside the box with the agent.
func (s *Store) Start(id string, wanted Instance) (Instance, error) {
	wanted.ID = id
	wanted.Started = time.Now().Unix()
	for _, each := range []string{s.Workspace(id), s.State(id), s.threadsDir(id)} {
		if err := os.MkdirAll(each, 0o755); err != nil {
			return Instance{}, err
		}
	}
	if err := fill(s.Workspace(id), wanted.Repo); err != nil {
		return Instance{}, err
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	s.held[id] = wanted
	s.threads[id] = map[string]Thread{}
	return wanted, s.write(wanted)
}

func (s *Store) Get(id string) (Instance, bool) {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.held[id]
	return held, found
}

// List is every instance, newest first.
func (s *Store) List() []Instance {
	s.guard.Lock()
	defer s.guard.Unlock()
	all := make([]Instance, 0, len(s.held))
	for _, held := range s.held {
		all = append(all, held)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].Started > all[b].Started })
	return all
}

func (s *Store) write(held Instance) error {
	return writeJSON(filepath.Join(s.Dir(held.ID), "instance.json"), held)
}

// load reads one instance directory, lifting one written before threads existed.
func (s *Store) load(dir string) error {
	held, err := readInstance(dir)
	if err != nil {
		if held, err = lift(dir); err != nil {
			return err
		}
	}
	threads, err := readThreads(filepath.Join(dir, "threads"))
	if err != nil {
		return err
	}
	// A workspace is a repository. One written before that was true becomes one
	// now rather than failing the next turn taken on it, which is what a
	// workspace with nothing to diff against does.
	if err := repair(filepath.Join(dir, "workspace")); err != nil {
		return err
	}
	s.held[held.ID] = held
	s.threads[held.ID] = threads
	return nil
}

func readInstance(dir string) (Instance, error) {
	text, err := os.ReadFile(filepath.Join(dir, "instance.json"))
	if err != nil {
		return Instance{}, err
	}
	var held Instance
	err = json.Unmarshal(text, &held)
	return held, err
}

// fill makes the workspace a repository. With a repository named it is that
// one, cloned once here rather than in every turn, so a credential for it stays
// outside the box with the agent. With none it is an empty repository, because
// a turn says what it changed by diffing the workspace, and a directory that is
// not a repository has nothing to diff against.
func fill(workspace, repo string) error {
	if repo == "" {
		return run(workspace, "init", "--quiet", ".")
	}
	clone := exec.Command("git", "clone", "--quiet", repo, ".")
	clone.Dir = workspace
	if output, err := clone.CombinedOutput(); err != nil {
		return fmt.Errorf("that repository could not be cloned: %s", output)
	}
	return nil
}

// repair makes an existing workspace a repository if it is not one already. A
// directory that is not there belongs to an instance that is not there either,
// and is left alone.
func repair(workspace string) error {
	if _, err := os.Stat(workspace); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err == nil {
		return nil
	}
	return run(workspace, "init", "--quiet", ".")
}

func run(at string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = at
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, output)
	}
	return nil
}
