package instance

import (
	"fmt"
	"log/slog"

	"github.com/getlatentic/gusto/internal/roomlog"
)

// What happened in a room is written to its event log in the same step as the change, under the store's guard, so the order of
// the log is the order of the changes and a sequence number is never given to a change that did not happen. People watching a
// room follow the log.

// The kinds of event a room's log holds. A turn event is any change to a turn: it was said, started, finished or stopped.
const (
	KindTurn   = "turn"
	KindNote   = "note"
	KindPeople = "people"
)

// eventsFile is where a room keeps its event log, in the room's own directory.
const eventsFile = "events.log"

// ErrNoRoom is the answer to asking for the events of something that is not a room.
var ErrNoRoom = fmt.Errorf("no such room")

// record adds an event to a room's log and tells the watchers. The guard is held. A log that cannot be written is logged and
// the room carries on, since a change that was made is not undone by being unable to announce it.
func (s *Store) record(id, thread, kind, ref string) {
	held, found := s.held[id]
	if !found || !held.Room {
		return
	}
	if _, err := s.events.Append(id, roomlog.Event{Kind: kind, Thread: thread, Ref: ref}); err != nil {
		slog.Warn("a room's event could not be recorded", "room", id, "error", err)
	}
	s.wake(id)
}

// wake tells every watcher that a room's log moved. The guard is held, so a watcher only notes it and returns.
func (s *Store) wake(id string) {
	for _, watch := range s.watchers {
		watch(id)
	}
}

// Record adds an event to a room's log for a change that is not the store's own, such as a turn starting or ending.
func (s *Store) Record(id, thread, kind, ref string) {
	s.guard.Lock()
	defer s.guard.Unlock()
	s.record(id, thread, kind, ref)
}

// OnRoomEvent has watch called with a room's id whenever an event is added to its log, and when the room is removed. It is called
// with the store's guard held, so it must not wait and must not use the store.
func (s *Store) OnRoomEvent(watch func(room string)) {
	s.guard.Lock()
	defer s.guard.Unlock()
	s.watchers = append(s.watchers, watch)
}

// RoomEvents is the events in a room's log after a sequence number, at most limit of them, and where its stream stands.
func (s *Store) RoomEvents(id string, after uint64, limit int) (roomlog.Slice, error) {
	if !s.isRoom(id) {
		return roomlog.Slice{}, ErrNoRoom
	}
	return s.events.After(id, after, limit)
}

// RoomHead is where a room's event stream stands: the epoch it is in and the sequence number of its newest event.
func (s *Store) RoomHead(id string) (roomlog.Position, error) {
	if !s.isRoom(id) {
		return roomlog.Position{}, ErrNoRoom
	}
	return s.events.Head(id)
}

func (s *Store) isRoom(id string) bool {
	held, found := s.Get(id)
	return found && held.Room
}
