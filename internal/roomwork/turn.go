package roomwork

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Turn is one agent's turn working on a copy of a room's folder.
type Turn struct {
	hub   *Hub
	room  Room
	agent string
	dest  string
	// begin is the number of merges the room had when this turn began. A file a later merge wrote was not in the copy.
	begin uint64
	// shared is the folder's files as the copy was made from them, and copied the copy's files as they were made. A file is the
	// turn's own change when the copy no longer matches copied, and somebody else's when the folder no longer matches shared.
	shared tree
	copied tree
	once   sync.Once
	closed atomic.Bool
}

// ErrClosed is the answer to merging a turn that has been closed: its copy is gone, and an empty copy would read as every file
// deleted.
var ErrClosed = errors.New("the turn is closed")

// Begin copies the room's folder to dest, which exists and is empty, for a turn of the named agent. The copy is made without
// stopping other turns, and a file another turn merges while it is made is either in the copy as it was or as it became, never
// half of each. A folder over the hub's limits is refused with a *TooBigError.
func (h *Hub) Begin(ctx context.Context, in Room, dest, agent string) (*Turn, error) {
	var err error
	if in.Folder, err = filepath.Abs(in.Folder); err != nil {
		return nil, err
	}
	if in.Meta, err = filepath.Abs(in.Meta); err != nil {
		return nil, err
	}
	turn := &Turn{hub: h, room: in, agent: agent, dest: dest}
	h.join(in, turn)
	if err := turn.start(ctx); err != nil {
		turn.Close()
		return nil, err
	}
	return turn, nil
}

func (t *Turn) start(ctx context.Context) error {
	if err := t.hub.heal(ctx, t.room); err != nil {
		return err
	}
	var err error
	if t.shared, err = clone(ctx, t.room.Folder, t.dest, t.hub.limits); err != nil {
		return err
	}
	if t.copied, err = scan(t.dest); err != nil {
		return err
	}
	select {
	case <-time.After(timestampGrain):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// timestampGrain is longer than the coarsest file time a file system keeps (a Linux kernel tick), so a file the agent writes
// after Begin returns is dated after every file in the copy, and a write is told from the copy it began as.
const timestampGrain = 12 * time.Millisecond

// Close ends the turn and empties its copy's folder. It is safe to call more than once, and a turn that was not merged has
// changed nothing in the room's folder.
func (t *Turn) Close() {
	t.once.Do(func() {
		t.closed.Store(true)
		_ = os.RemoveAll(t.dest)
		_ = os.MkdirAll(t.dest, 0o755)
		t.hub.leave(t.room, t)
	})
}
