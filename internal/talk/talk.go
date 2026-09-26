// Package talk is a conversation with a harness.
//
// A job is one shot: a brief, a gate, a verdict. A conversation is turns against
// something that stays. So a session holds what must survive between turns — the
// working directory and the harness's own memory of the conversation — and each
// turn is an ordinary job that mounts them instead of cloning a repository.
//
// Nothing here is a second way to run things. A turn is queued, sandboxed,
// streamed and recorded exactly as any other job is.
package talk

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

// Session is what stays between turns.
type Session struct {
	ID      string `json:"id"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	Repo    string `json:"repo,omitempty"`
	// Remembered is the harness's own name for this conversation, learned from
	// the first turn. A harness resumes by being handed it back.
	Remembered string   `json:"remembered,omitempty"`
	Turns      []string `json:"turns"`
	Started    int64    `json:"startedAt"`
}

// Sessions is every conversation under one directory.
type Sessions struct {
	root  string
	guard sync.Mutex
	held  map[string]Session
}

func Open(root string) (*Sessions, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	sessions := &Sessions{root: root, held: map[string]Session{}}
	listing, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range listing {
		if !entry.IsDir() {
			continue
		}
		session, err := read(filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		sessions.held[session.ID] = session
	}
	return sessions, nil
}

func (s *Sessions) Root() string { return s.root }

// Dir is where a session keeps what stays.
func (s *Sessions) Dir(id string) string { return filepath.Join(s.root, id) }

// Workspace is the directory a harness works in, across every turn.
func (s *Sessions) Workspace(id string) string { return filepath.Join(s.Dir(id), "workspace") }

// State is where a harness keeps its own memory of the conversation. It is
// outside the workspace because that is where harnesses put it: opencode keeps
// sessions under its data directory, not beside the files it edits.
func (s *Sessions) State(id string) string { return filepath.Join(s.Dir(id), "state") }

// Start makes a session, and fills its workspace from a repository when one is
// given. Cloning happens here rather than in a turn, so it happens once and so
// a repository credential stays outside the box with the agent.
func (s *Sessions) Start(id string, session Session) (Session, error) {
	session.ID = id
	session.Started = time.Now().Unix()
	for _, each := range []string{s.Workspace(id), s.State(id)} {
		if err := os.MkdirAll(each, 0o755); err != nil {
			return Session{}, err
		}
	}
	if session.Repo != "" {
		clone := exec.Command("git", "clone", "--quiet", session.Repo, ".")
		clone.Dir = s.Workspace(id)
		if output, err := clone.CombinedOutput(); err != nil {
			return Session{}, fmt.Errorf("that repository could not be cloned: %s", output)
		}
	}
	if err := s.keep(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// Said records a turn, and the harness's own name for the conversation the
// first time it says one.
func (s *Sessions) Said(id, job, remembered string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	session, found := s.held[id]
	if !found {
		return fmt.Errorf("no session called %s", id)
	}
	if job != "" {
		session.Turns = append(session.Turns, job)
	}
	if remembered != "" {
		session.Remembered = remembered
	}
	s.held[id] = session
	return s.write(session)
}

func (s *Sessions) Get(id string) (Session, bool) {
	s.guard.Lock()
	defer s.guard.Unlock()
	session, found := s.held[id]
	return session, found
}

func (s *Sessions) List() []Session {
	s.guard.Lock()
	defer s.guard.Unlock()
	all := make([]Session, 0, len(s.held))
	for _, session := range s.held {
		all = append(all, session)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].Started > all[b].Started })
	return all
}

func (s *Sessions) keep(session Session) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	s.held[session.ID] = session
	return s.write(session)
}

func (s *Sessions) write(session Session) error {
	text, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir(session.ID), "session.json"), append(text, '\n'), 0o644)
}

func read(dir string) (Session, error) {
	text, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return Session{}, err
	}
	var session Session
	err = json.Unmarshal(text, &session)
	return session, err
}
