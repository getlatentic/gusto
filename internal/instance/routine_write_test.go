package instance

import (
	"os"
	"runtime"
	"testing"
)

// An add whose routines.json cannot be written leaves no thread named after a routine that is not there.
// spec/Routines.before-undo.cfg is the trace.
func TestARoutineThatCannotBeKeptLeavesNoThread(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a folder's mode stops writes only for a person who is not root, on Unix")
	}
	store := opened(t)
	held, err := store.Start("20261103-0900-work", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	before := len(store.Threads(held.ID))
	if err := os.Chmod(store.Dir(held.ID), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(store.Dir(held.ID), 0o755) })
	if _, err := store.AddRoutine(held.ID, Routine{Title: "Morning", Prompt: "go", Every: 1}); err == nil {
		t.Fatal("routines.json could not be written, and the routine was taken")
	}
	if got := len(store.Threads(held.ID)); got != before {
		t.Errorf("the failed add left %d threads where there were %d", got, before)
	}
	os.Chmod(store.Dir(held.ID), 0o755)
	again, err := Open(store.Root())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(again.Threads(held.ID)); got != before {
		t.Errorf("read back, the failed add left %d threads where there were %d", got, before)
	}
}
