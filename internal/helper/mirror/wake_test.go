package mirror

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestAWakeStartsARunAtOnceAndABurstIsOneRun(t *testing.T) {
	p := newPair(t)
	engine := p.fresh()
	ctx, stop := context.WithCancel(context.Background())
	var runs atomic.Int32
	wake := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		engine.Keep(ctx, time.Hour, time.Hour, wake, func(*Report, error) { runs.Add(1) })
		close(done)
	}()
	waitFor(t, "the first run", func() bool { return runs.Load() == 1 })
	for range 30 {
		pulse(wake)
		time.Sleep(20 * time.Millisecond)
	}
	waitFor(t, "a run after the burst", func() bool { return runs.Load() >= 2 })
	time.Sleep(2 * settleTime)
	if got := runs.Load(); got != 2 {
		t.Errorf("a burst of 30 saves made %d runs after the first, want 1", got-1)
	}
	stop()
	<-done
}

func TestAWakeDuringTheSettlingEndsWithTheContext(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	wake := make(chan struct{}, 1)
	go func() { time.Sleep(50 * time.Millisecond); stop() }()
	if settle(ctx, wake) {
		t.Error("settle said to go on after the context ended")
	}
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("no %s in 5 seconds", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func woken(wake <-chan struct{}, within time.Duration) bool {
	select {
	case <-wake:
		return true
	case <-time.After(within):
		return false
	}
}

func TestTheWatcherWakesOnASaveInTheFolderAndInOneMadeLater(t *testing.T) {
	root := t.TempDir()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	wake, err := Watch(ctx, root, NewIgnore(Defaults...), nil)
	if err != nil {
		t.Skip(err)
	}
	time.Sleep(300 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("1"), 0o600)
	if !woken(wake, 3*time.Second) {
		t.Fatal("a save in the folder did not wake the watcher")
	}
	os.MkdirAll(filepath.Join(root, "deep", "er"), 0o700)
	woken(wake, 2*time.Second)
	time.Sleep(500 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "deep", "er", "b.txt"), []byte("2"), 0o600)
	if !woken(wake, 3*time.Second) {
		t.Fatal("a save in a folder made after the watch began did not wake the watcher")
	}
}

func TestTheWatcherIsSilentForWhatTheSyncLeavesOut(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "node_modules"), 0o700)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	wake, err := Watch(ctx, root, NewIgnore(Defaults...), nil)
	if err != nil {
		t.Skip(err)
	}
	time.Sleep(300 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "node_modules", "x.js"), []byte("1"), 0o600)
	os.WriteFile(filepath.Join(root, ".gusto-pull-123"), []byte("1"), 0o600)
	os.WriteFile(filepath.Join(root, ".DS_Store"), []byte("1"), 0o600)
	if woken(wake, time.Second) {
		t.Fatal("a file the sync leaves out woke the watcher")
	}
}
