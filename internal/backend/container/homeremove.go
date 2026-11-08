package container

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tendEvery is how often the homes of people who left are looked for.
const tendEvery = time.Minute

// ErrHomeBusy is a home a turn is running in, which is left for the next look and cannot be reset now.
var ErrHomeBusy = errors.New("a turn is running in this home")

// computerID is what a computer's id looks like, so that nothing that is not one names a place outside the homes.
var computerID = regexp.MustCompile(`^[0-9a-f]{8}$`)

// tend removes the homes that were asked to be removed, once now and then every tendEvery, until ctx ends. done closes when it
// has. A person who leaves the organization has a mark made for their computer in each workspace (`<id>.retire`, beside the
// homes), which is how the organization's server, which keeps the files, is told by gusto cloud, which does not.
func (h *homes) tend(ctx context.Context) (done <-chan struct{}) {
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		for {
			h.removeMarked()
			select {
			case <-ctx.Done():
				return
			case <-time.After(h.tending):
			}
		}
	}()
	return ended
}

func (h *homes) removeMarked() {
	entries, err := os.ReadDir(h.root)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Error("the homes asked to be removed could not be listed", "error", err)
		}
		return
	}
	for _, entry := range entries {
		id, marked := strings.CutSuffix(entry.Name(), ".retire")
		if !marked || !computerID.MatchString(id) {
			continue
		}
		h.removeMarkedHome(id)
	}
}

func (h *homes) removeMarkedHome(id string) {
	switch err := h.remove(id); {
	case errors.Is(err, ErrHomeBusy):
	case err != nil:
		slog.Error("a home asked to be removed could not be", "home", id, "error", err)
	default:
		if err := os.Remove(h.markFile(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.Error("a removed home's mark could not be cleared", "home", id, "error", err)
		}
	}
}

// remove deletes a computer's home and what is kept of its size. Files an agent left as another user may not be removable by
// this server, so what could not be removed is moved whole to the trash beside the homes, for a job that runs as root to empty.
func (h *homes) remove(id string) error {
	if err := h.startRemoving(id); err != nil {
		return err
	}
	defer h.doneRemoving(id)
	err := os.RemoveAll(h.dir(id))
	if errors.Is(err, fs.ErrPermission) {
		err = h.trash(id)
	}
	if err != nil {
		return err
	}
	if err := os.Remove(h.usageFile(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	os.Remove(filepath.Dir(h.usageFile(id)))
	return nil
}

func (h *homes) startRemoving(id string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active[id] > 0 {
		return ErrHomeBusy
	}
	h.removing[id] = true
	return nil
}

func (h *homes) doneRemoving(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.removing, id)
}

// trash moves a home that could not be removed to the trash, whole.
func (h *homes) trash(id string) error {
	trash := filepath.Join(h.root, "trash")
	if err := os.MkdirAll(trash, 0o700); err != nil {
		return err
	}
	to := filepath.Join(trash, id+"-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.Rename(h.dir(id), to); err != nil {
		return err
	}
	slog.Warn("a home held files this process may not remove, and went to the trash", "home", id, "trash", to)
	return nil
}
