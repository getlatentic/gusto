package filelock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// A file one holder holds is refused to a second until the first lets go, and Wait takes it then.
func TestAHeldFileIsRefusedUntilItIsLetGo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "lock")
	first, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Try(path); !errors.Is(err, ErrHeld) {
		t.Fatalf("a held file was given to a second holder: %v", err)
	}
	short, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if _, err := Wait(short, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait on a held file said %v", err)
	}
	go func() { time.Sleep(80 * time.Millisecond); first.Release() }()
	second, err := Wait(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	second.Release()
}
