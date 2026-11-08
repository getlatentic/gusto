package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Thread is one conversation with an instance. An instance has as many as
// somebody wants, and they share its folder, which is why only one turn of
// one thread runs at a time.
type Thread struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
	Title    string `json:"title"`
	// Remembered is the agent's own name for this conversation, learned from
	// the first turn and handed back to resume it. It belongs to the thread
	// rather than the instance because an agent keeps one per conversation.
	Remembered string `json:"remembered,omitempty"`
	// Dir is the folder this conversation works in, from the root of the instance's folder. It is empty for the root.
	Dir     string   `json:"dir,omitempty"`
	Turns   []string `json:"turns"`
	Started int64    `json:"startedAt"`
	// Sessions is each room agent's own name for this conversation: in a room every agent keeps a conversation of its own.
	Sessions map[string]string `json:"sessions,omitempty"`
	// Notes are what people said in a room that woke no agent.
	Notes []Note `json:"notes,omitempty"`
	// Seen is when each person last looked at this conversation, so a room can say what they have not read.
	Seen map[string]int64 `json:"seen,omitempty"`
}

// NewThread opens a conversation on an instance. Threads are numbered in the
// order they are opened, which is short enough to read in a URL and stable when
// a title changes.
func (s *Store) NewThread(id, title string) (Thread, error) { return s.NewThreadIn(id, title, "") }

// NewThreadIn opens a conversation that works in a folder of the instance's folder.
func (s *Store) NewThreadIn(id, title, dir string) (Thread, error) {
	s.guard.Lock()
	defer s.guard.Unlock()
	if _, found := s.held[id]; !found {
		return Thread{}, fmt.Errorf("no instance called %s", id)
	}
	return s.openIn(id, title, dir)
}

// open numbers a conversation, holds it and writes it. The caller holds the
// guard, or is Open, before anybody else can reach the store. The thread is
// held before it is written, so an instance whose directory refuses the write
// still has somewhere to talk for as long as this process runs.
func (s *Store) open(id, title string) (Thread, error) { return s.openIn(id, title, "") }

func (s *Store) openIn(id, title, dir string) (Thread, error) {
	if s.threads[id] == nil {
		s.threads[id] = map[string]Thread{}
	}
	thread := s.numbered(id, title, dir)
	s.threads[id][thread.ID] = thread
	return thread, s.writeThread(thread)
}

// numbered is a new conversation with the next number of an instance, not yet held or written.
func (s *Store) numbered(id, title, dir string) Thread {
	return Thread{ID: strconv.Itoa(s.next(id)), Instance: id, Title: title, Dir: dir, Started: time.Now().Unix()}
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

// next is one past the highest number in use, in memory or in the instance's threads folder. A file that cannot be read still
// has its number, because tasks name their conversation by it and a second conversation given it would be taken for the first.
func (s *Store) next(id string) int {
	highest := 0
	for name := range s.threads[id] {
		if number, err := strconv.Atoi(name); err == nil && number > highest {
			highest = number
		}
	}
	listing, _ := os.ReadDir(s.threadsDir(id))
	for _, entry := range listing {
		number, _, _ := strings.Cut(entry.Name(), ".")
		if number, err := strconv.Atoi(number); err == nil && number > highest {
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

// amend changes a copy of a thread, writes the copy, and only then holds it, so the thread in memory is never ahead of its file
// and a write that fails changes nothing. edit says whether it changed the thread. The caller holds the guard.
func (s *Store) amend(id, thread string, edit func(*Thread) bool) (Thread, error) {
	held, found := s.threads[id][thread]
	if !found {
		return Thread{}, fmt.Errorf("no thread %s on %s", thread, id)
	}
	next := held.clone()
	if !edit(&next) {
		return held, nil
	}
	if err := s.writeThread(next); err != nil {
		return held, err
	}
	s.threads[id][next.ID] = next
	return next, nil
}

func (t Thread) clone() Thread {
	t.Turns = slices.Clone(t.Turns)
	t.Notes = slices.Clone(t.Notes)
	t.Sessions = maps.Clone(t.Sessions)
	t.Seen = maps.Clone(t.Seen)
	return t
}

// Said records a turn on a thread, and the agent's own name for the conversation the first time it says one. The turn is an
// event of the room only when the thread was written.
func (s *Store) Said(id, thread, task, remembered string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	_, err := s.amend(id, thread, func(held *Thread) bool {
		if task != "" {
			held.Turns = append(held.Turns, task)
		}
		if remembered != "" {
			held.Remembered = remembered
		}
		return task != "" || remembered != ""
	})
	if err == nil && task != "" {
		s.record(id, thread, KindTurn, task)
	}
	return err
}

// Name gives a thread a title when it has none. Somebody who named a thread
// keeps their name; one opened without is named by what was first said in it,
// which is how a list of conversations is told apart.
func (s *Store) Name(id, thread, title string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	_, err := s.amend(id, thread, func(held *Thread) bool {
		if held.Title != "" || title == "" {
			return false
		}
		held.Title = title
		return true
	})
	return err
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

// forgetThread lets go of a thread that was opened for something that could not be kept.
func (s *Store) forgetThread(thread Thread) error {
	delete(s.threads[thread.Instance], thread.ID)
	err := os.Remove(filepath.Join(s.threadsDir(thread.Instance), thread.ID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// damaged is what a conversation is called when its file could not be read.
const damaged = "damaged conversation"

// readThreads reads an instance's conversations. A file that cannot be parsed, as a crash or a disk fault leaves one, is
// moved aside whole, and the conversation takes its number again with nothing said in it, so tasks that name the number still
// find a conversation and the old text is still there to read. Nothing is dropped without being logged.
func readThreads(dir, instance string) (map[string]Thread, error) {
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
		path := filepath.Join(dir, entry.Name())
		text, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("a conversation's file could not be read, so it is left out", "file", path, "error", err)
			continue
		}
		var thread Thread
		if json.Unmarshal(text, &thread) == nil && thread.ID != "" {
			threads[thread.ID] = thread
			continue
		}
		if mended, ok := setAsideThread(path, instance); ok {
			threads[mended.ID] = mended
		}
	}
	return threads, nil
}

// setAsideThread keeps a damaged conversation file beside its place and writes an empty conversation of the same number there.
func setAsideThread(path, instance string) (Thread, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return Thread{}, false
	}
	kept := fmt.Sprintf("%s.damaged-%d", path, time.Now().Unix())
	if err := os.Rename(path, kept); err != nil {
		slog.Warn("a damaged conversation file could not be set aside, so it is left out", "file", path, "error", err)
		return Thread{}, false
	}
	thread := Thread{ID: strings.TrimSuffix(filepath.Base(path), ".json"), Instance: instance, Title: damaged, Started: info.ModTime().Unix()}
	if err := writeJSON(path, thread); err != nil {
		slog.Warn("an empty conversation could not be written for a damaged one", "file", path, "error", err)
	}
	slog.Warn("a conversation's file was damaged: it is kept beside it and the conversation starts again empty", "file", path, "kept", kept)
	return thread, true
}
