// Package filelock holds a file for one process at a time, as one program does a piece of state that two would spoil. The
// operating system lets go of a held file when the process that holds it ends, however it ends, so no lock outlives its holder.
package filelock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ErrHeld says another holds the file.
var ErrHeld = errors.New("another program holds it")

// Held is a file this process holds until Release.
type Held struct{ file *os.File }

// Try holds the file at path, making it when it is not there, or says ErrHeld at once when another holds it.
func Try(path string) (*Held, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := hold(file); err != nil {
		file.Close()
		return nil, err
	}
	return &Held{file: file}, nil
}

// poll is how often Wait tries again.
const poll = 50 * time.Millisecond

// Wait holds the file at path once whoever holds it lets go, or says why it could not before ctx ended.
func Wait(ctx context.Context, path string) (*Held, error) {
	for {
		held, err := Try(path)
		if !errors.Is(err, ErrHeld) {
			return held, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(poll):
		}
	}
}

// Release lets go of the file.
func (h *Held) Release() error { return h.file.Close() }
