// Package roomwork lets the agents of one room work at the same time in the room's one folder.
//
// Each turn works on a copy of the folder, and what the turn changed is merged into the folder when the turn passes. A file
// two turns changed is not overwritten: the folder keeps the version that arrived first, the other is saved beside it under a
// name that says whose it is, and the merge says so. A turn that fails or is stopped merges nothing.
package roomwork

import (
	"context"
	"os"
	"sync"
	"time"
)

// Room is the folder agents share, and where the merge keeps its staging files. Meta is on the same file system as Folder, so
// a staged file is renamed into place and a reader never sees half a file.
type Room struct {
	Folder string
	Meta   string
}

// Hub hands out working copies of room folders and merges them back. It is safe for any number of turns at once.
type Hub struct {
	limits Limits
	mu     sync.Mutex
	rooms  map[string]*roomState
}

// roomState is what the hub knows of a room while a turn is working in it.
type roomState struct {
	// guard holds one token while a merge changes the folder. A merge checks files and then replaces them, and two merges
	// doing that to one file at once would lose one of them, so this is a lock around file system work, held for the files the
	// turn changed and no more. It is a channel so that a stopped turn does not wait for it.
	guard chan struct{}
	// The rest is guarded by Hub.mu.
	live    map[*Turn]struct{}
	seq     uint64
	healed  bool
	history history
}

// guardWait is the longest a merge waits for another merge into the same room. A merge is the changed files of one turn, so
// waiting longer means something is wrong.
const guardWait = 2 * time.Minute

// NewHub is a hub whose working copies are held to limits.
func NewHub(limits Limits) *Hub {
	return &Hub{limits: limits, rooms: map[string]*roomState{}}
}

// join adds a turn to its room's state, making the state if the room has none.
func (h *Hub) join(in Room, turn *Turn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.rooms[in.Folder]
	if state == nil {
		state = &roomState{guard: make(chan struct{}, 1), live: map[*Turn]struct{}{}, history: newHistory()}
		h.rooms[in.Folder] = state
	}
	turn.begin = state.seq
	state.live[turn] = struct{}{}
}

// leave takes a turn out of its room, and lets go of what the room kept for turns that began before every one still working.
func (h *Hub) leave(in Room, turn *Turn) {
	h.mu.Lock()
	state := h.rooms[in.Folder]
	if state == nil {
		h.mu.Unlock()
		return
	}
	delete(state.live, turn)
	oldest := state.seq
	for each := range state.live {
		oldest = min(oldest, each.begin)
	}
	if len(state.live) == 0 {
		delete(h.rooms, in.Folder)
	}
	spent := state.history.trim(oldest)
	idle := len(state.live) == 0
	h.mu.Unlock()
	for _, file := range spent {
		_ = os.Remove(file)
	}
	if idle {
		_ = os.RemoveAll(priorDir(in))
	}
}

// hold takes the room's merge guard, or gives up when the context ends or the wait does.
func (h *Hub) hold(ctx context.Context, in Room) (release func(), err error) {
	h.mu.Lock()
	state := h.rooms[in.Folder]
	h.mu.Unlock()
	ctx, stop := context.WithTimeout(ctx, guardWait)
	defer stop()
	select {
	case state.guard <- struct{}{}:
		return func() { <-state.guard }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// heal undoes a merge that a crash cut off, once for each time a room becomes busy. It runs under the guard.
func (h *Hub) heal(in Room) error {
	h.mu.Lock()
	state := h.rooms[in.Folder]
	done := state.healed
	h.mu.Unlock()
	if done {
		return nil
	}
	if err := recoverMerge(in); err != nil {
		return err
	}
	// No turn is working from before this, so no version a merge kept is needed.
	_ = os.RemoveAll(priorDir(in))
	h.mu.Lock()
	state.healed = true
	h.mu.Unlock()
	return nil
}
