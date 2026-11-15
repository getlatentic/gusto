package instance

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// removed is where a removed instance's directory goes. It is inside the
// store's root so that removing never crosses a filesystem, and its name starts
// with a dot so that opening the store passes over it.
const removed = ".removed"

// Remove takes an instance out of the store. Its directory is moved aside, not
// deleted: the folder is somebody's work, and a person who removed the wrong
// one can move it back. The tasks it ran are records of their own and stay.
func (s *Store) Remove(id string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	if _, found := s.held[id]; !found {
		return fmt.Errorf("no instance called %s", id)
	}
	aside := filepath.Join(s.root, removed)
	if err := os.MkdirAll(aside, 0o755); err != nil {
		return err
	}
	to := filepath.Join(aside, fmt.Sprintf("%s-%d", id, time.Now().Unix()))
	if err := os.Rename(s.Dir(id), to); err != nil {
		return err
	}
	s.drop(id)
	s.gone[id] = struct{}{}
	// The room's log moved with its directory. Its copy in memory goes too, or a room made under the same name would read the
	// window of this one.
	if err := s.events.Drop(id); err != nil {
		slog.Warn("a removed room's event log could not be dropped", "room", id, "error", err)
	}
	s.wake(id)
	return nil
}

// wasRemoved says whether an instance with this id was removed.
func (s *Store) wasRemoved(id string) bool {
	s.guard.Lock()
	defer s.guard.Unlock()
	_, gone := s.gone[id]
	return gone
}

// readGone is the ids of the instances set aside under root. A set-aside directory is named for its id and the second it was
// removed, so an id is what is left when the last dash and the digits after it are taken off.
func readGone(root string) map[string]struct{} {
	gone := map[string]struct{}{}
	listing, _ := os.ReadDir(filepath.Join(root, removed))
	for _, entry := range listing {
		name := entry.Name()
		at := strings.LastIndex(name, "-")
		if at <= 0 {
			continue
		}
		if _, err := strconv.ParseInt(name[at+1:], 10, 64); err == nil {
			gone[name[:at]] = struct{}{}
		}
	}
	return gone
}

func hidden(name string) bool { return strings.HasPrefix(name, ".") }
