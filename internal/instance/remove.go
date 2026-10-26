package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// removed is where a removed instance's directory goes. It is inside the
// store's root so that removing never crosses a filesystem, and its name starts
// with a dot so that opening the store passes over it.
const removed = ".removed"

// Remove takes an instance out of the store. Its directory is moved aside, not
// deleted: the workspace is somebody's work, and a person who removed the wrong
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
	s.wake(id)
	return nil
}

func hidden(name string) bool { return strings.HasPrefix(name, ".") }
