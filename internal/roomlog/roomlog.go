// Package roomlog is the durable, bounded event log of each room.
//
// A room's log is the stream a live view follows: every event has a sequence number that is allocated when it is
// appended, so the order of the log is the order the changes were made in. The numbers belong to an epoch, a random word
// written at the head of the log's file, which changes if the log is lost and started again; a position is only
// meaningful with the epoch it was read under.
//
// The log keeps a window of the newest events. A reader that asks for events from before the window gets none, and is told
// so, because it can no longer be brought up to date by replay and needs a snapshot.
package roomlog

import (
	"container/list"
	"errors"
	"sync"
)

// Event is one thing that happened in a room: what kind of thing, in which conversation, and which turn or note it was. The
// log holds descriptors and not rendered pages, so it stays small and what a viewer is shown is always drawn from the state
// as it is.
type Event struct {
	Seq    uint64 `json:"s"`
	At     int64  `json:"t"`
	Kind   string `json:"k"`
	Thread string `json:"h,omitempty"`
	Ref    string `json:"r,omitempty"`
}

// Position is a place in a room's stream.
type Position struct {
	Epoch string
	Seq   uint64
}

// Slice is the events after a sequence number, in order, with where the stream stands now. Gap says the events the reader
// needed are no longer all in the window, so Events is empty and the reader must start again from a snapshot.
type Slice struct {
	Events []Event
	Head   Position
	Gap    bool
}

// Options sizes a Set.
type Options struct {
	// Window is how many of a room's newest events are kept.
	Window int
	// MaxLoaded is how many rooms' logs are held in memory at once. A room whose log is evicted is read from its file again
	// when it is next used.
	MaxLoaded int
}

func (o Options) withDefaults() Options {
	if o.Window <= 0 {
		o.Window = 512
	}
	if o.MaxLoaded <= 0 {
		o.MaxLoaded = 2048
	}
	return o
}

// ErrClosed is the answer to using a Set after it was closed.
var ErrClosed = errors.New("the room log is closed")

// Set is the logs of many rooms. Each is a file at the path pathOf names.
type Set struct {
	pathOf func(room string) string
	opts   Options

	mu     sync.Mutex
	loaded map[string]*list.Element
	order  *list.List
	closed bool
}

// entry is a room's log in memory, pinned while it is in use so that it is not evicted under a caller.
type entry struct {
	room string
	pins int
	once sync.Once
	log  *roomLog
	err  error
}

// NewSet makes a Set whose room logs are at pathOf(room).
func NewSet(pathOf func(room string) string, opts Options) *Set {
	return &Set{pathOf: pathOf, opts: opts.withDefaults(), loaded: map[string]*list.Element{}, order: list.New()}
}

// use runs f on a room's log. The log is read from its file the first time and kept until it is the least recently used of
// too many.
func (s *Set) use(room string, f func(*roomLog)) error {
	held, err := s.pin(room)
	if err != nil {
		return err
	}
	defer s.unpin(held)
	held.once.Do(func() { held.log, held.err = openLog(s.pathOf(room), s.opts.Window) })
	if held.err != nil {
		s.forget(held)
		return held.err
	}
	f(held.log)
	return nil
}

func (s *Set) pin(room string) (*entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if at, found := s.loaded[room]; found {
		s.order.MoveToFront(at)
		held := at.Value.(*entry)
		held.pins++
		return held, nil
	}
	held := &entry{room: room, pins: 1}
	s.loaded[room] = s.order.PushFront(held)
	s.evict()
	return held, nil
}

// evict drops the least recently used logs that nobody is using, down to the bound.
func (s *Set) evict() {
	for at := s.order.Back(); at != nil && s.order.Len() > s.opts.MaxLoaded; {
		previous := at.Prev()
		if held := at.Value.(*entry); held.pins == 0 {
			s.order.Remove(at)
			delete(s.loaded, held.room)
		}
		at = previous
	}
}

func (s *Set) unpin(held *entry) {
	s.mu.Lock()
	held.pins--
	s.evict()
	s.mu.Unlock()
}

// forget takes an entry that could not be read out of the Set, so the next use tries the file again.
func (s *Set) forget(held *entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if at, found := s.loaded[held.room]; found && at.Value.(*entry) == held {
		s.order.Remove(at)
		delete(s.loaded, held.room)
	}
}

// Append adds an event to a room's log and gives it the next sequence number. The event is in the log's file before Append
// returns. If the file cannot be written the event is still kept in memory and the log starts a new epoch, since the numbers
// it hands out can no longer be promised across a restart; the error says so.
func (s *Set) Append(room string, event Event) (Event, error) {
	var out Event
	var written error
	if err := s.use(room, func(l *roomLog) { out, written = l.append(event) }); err != nil {
		return Event{}, err
	}
	return out, written
}

// After gives the events with a sequence number above seq, at most limit of them, and where the stream stands.
func (s *Set) After(room string, seq uint64, limit int) (Slice, error) {
	var out Slice
	err := s.use(room, func(l *roomLog) { out = l.after(seq, limit) })
	return out, err
}

// Head is where a room's stream stands.
func (s *Set) Head(room string) (Position, error) {
	var out Position
	err := s.use(room, func(l *roomLog) { out = l.position() })
	return out, err
}

// Drop forgets a room's log and removes its file.
func (s *Set) Drop(room string) error {
	s.mu.Lock()
	if at, found := s.loaded[room]; found {
		s.order.Remove(at)
		delete(s.loaded, room)
	}
	s.mu.Unlock()
	return removeLog(s.pathOf(room))
}

// Close stops the Set from being used. What is written stays.
func (s *Set) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}
