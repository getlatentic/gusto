package instance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Thread is one conversation with an instance. An instance has as many as
// somebody wants, and they share its workspace, which is why only one turn of
// one thread runs at a time.
type Thread struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
	Title    string `json:"title"`
	// Remembered is the harness's own name for this conversation, learned from
	// the first turn and handed back to resume it. It belongs to the thread
	// rather than the instance because a harness keeps one per conversation.
	Remembered string   `json:"remembered,omitempty"`
	Turns      []string `json:"turns"`
	Started    int64    `json:"startedAt"`
}

// NewThread opens a conversation on an instance. Threads are numbered in the
// order they are opened, which is short enough to read in a URL and stable when
// a title changes.
func (s *Store) NewThread(id, title string) (Thread, error) {
	s.guard.Lock()
	defer s.guard.Unlock()
	if _, found := s.held[id]; !found {
		return Thread{}, fmt.Errorf("no instance called %s", id)
	}
	if s.threads[id] == nil {
		s.threads[id] = map[string]Thread{}
	}
	thread := Thread{
		ID:       strconv.Itoa(s.next(id)),
		Instance: id,
		Title:    title,
		Started:  time.Now().Unix(),
	}
	s.threads[id][thread.ID] = thread
	return thread, s.writeThread(thread)
}

// next is one past the highest number in use, so a deleted thread's number is
// not handed to a second conversation.
func (s *Store) next(id string) int {
	highest := 0
	for name := range s.threads[id] {
		if number, err := strconv.Atoi(name); err == nil && number > highest {
			highest = number
		}
	}
	return highest + 1
}

func (s *Store) Thread(id, thread string) (Thread, bool) {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.threads[id][thread]
	return held, found
}

// Threads is every conversation on an instance, oldest first, which is the
// order they were opened in.
func (s *Store) Threads(id string) []Thread {
	s.guard.Lock()
	defer s.guard.Unlock()
	all := make([]Thread, 0, len(s.threads[id]))
	for _, held := range s.threads[id] {
		all = append(all, held)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].Started < all[b].Started })
	return all
}

// Said records a turn on a thread, and the harness's own name for the
// conversation the first time it says one.
func (s *Store) Said(id, thread, job, remembered string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.threads[id][thread]
	if !found {
		return fmt.Errorf("no thread %s on %s", thread, id)
	}
	if job != "" {
		held.Turns = append(held.Turns, job)
	}
	if remembered != "" {
		held.Remembered = remembered
	}
	s.threads[id][held.ID] = held
	return s.writeThread(held)
}

func (s *Store) writeThread(thread Thread) error {
	return writeJSON(filepath.Join(s.threadsDir(thread.Instance), thread.ID+".json"), thread)
}

func readThreads(dir string) (map[string]Thread, error) {
	threads := map[string]Thread{}
	listing, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return threads, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range listing {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		text, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var thread Thread
		if json.Unmarshal(text, &thread) == nil && thread.ID != "" {
			threads[thread.ID] = thread
		}
	}
	return threads, nil
}
