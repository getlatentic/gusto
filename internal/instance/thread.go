package instance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	return s.open(id, title)
}

// open numbers a conversation, holds it and writes it. The caller holds the
// guard, or is Open, before anybody else can reach the store. The thread is
// held before it is written, so an instance whose directory refuses the write
// still has somewhere to talk for as long as this process runs.
func (s *Store) open(id, title string) (Thread, error) {
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

// mend opens the first conversation on an instance that has none, which is how
// the two-step start left every instance it made. It runs once, as the store
// loads, and writes the thread to disk, so the next load finds it there and
// nothing that reads the store has to allow for an instance without one.
//
// A directory that cannot be written keeps the thread in memory only. The
// instance is still listed and still leads into a conversation; the write is
// tried again when the store next loads, and anything said on the thread meets
// the same refusal, reported where it is said.
func (s *Store) mend(id string) {
	if len(s.threads[id]) > 0 {
		return
	}
	s.open(id, "")
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
	sort.Slice(all, func(a, b int) bool { return openedBefore(all[a], all[b]) })
	return all
}

// openedBefore orders threads by when they were opened. A second is coarse
// enough for two to share one, as the thread Start opens and the next one
// often do, and their numbers still say which came first.
func openedBefore(a, b Thread) bool {
	if a.Started != b.Started {
		return a.Started < b.Started
	}
	first, _ := strconv.Atoi(a.ID)
	second, _ := strconv.Atoi(b.ID)
	return first < second
}

// Latest is the conversation an instance was last holding: the one opened last.
// Every instance the store holds has one, because starting an instance opens it
// and loading one mends it, so only an id the store does not hold gets the zero
// Thread.
func (s *Store) Latest(id string) Thread {
	threads := s.Threads(id)
	if len(threads) == 0 {
		return Thread{}
	}
	return threads[len(threads)-1]
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

// Name gives a thread a title when it has none. Somebody who named a thread
// keeps their name; one opened without is named by what was first said in it,
// which is how a list of conversations is told apart.
func (s *Store) Name(id, thread, title string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.threads[id][thread]
	if !found {
		return fmt.Errorf("no thread %s on %s", thread, id)
	}
	if held.Title != "" || title == "" {
		return nil
	}
	held.Title = title
	s.threads[id][held.ID] = held
	return s.writeThread(held)
}

// TitleOf is the start of what was said, short enough for a list: the first
// line, cut at a word.
func TitleOf(said string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(said), "\n")
	words := strings.Fields(line)
	const most = 48
	title := ""
	for _, word := range words {
		if len(title)+len(word)+1 > most {
			return title + "…"
		}
		title = strings.TrimSpace(title + " " + word)
	}
	return title
}

func (s *Store) writeThread(thread Thread) error {
	if err := os.MkdirAll(s.threadsDir(thread.Instance), 0o755); err != nil {
		return err
	}
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
