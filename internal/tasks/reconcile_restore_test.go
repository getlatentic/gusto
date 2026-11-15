package tasks

import (
	"os"
	"testing"

	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/taskindex/taskindextest"
)

// restoredFromBackup is a root whose one task says it is still running, and a store whose row for it says it passed: the
// files were put back from a backup that was taken before the task ended.
func restoredFromBackup(t *testing.T) (string, *taskindextest.Fake) {
	t.Helper()
	root := t.TempDir()
	file := Record{ID: "a-task", State: Running, Queued: 10, Started: 11, Brief: "x"}
	if err := os.MkdirAll(file.Dir(root).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := write(file.Dir(root), file); err != nil {
		t.Fatal(err)
	}
	store := taskindextest.New()
	ended := RowOf(file)
	ended.State, ended.Ended = string(Passed), 20
	if err := store.Upsert(background, ended); err != nil {
		t.Fatal(err)
	}
	return root, store
}

func TestARebuildLeavesARowThatIsAheadOfItsFileAndSaysSo(t *testing.T) {
	root, store := restoredFromBackup(t)
	got, err := Rebuild(background, store, root, false, nil)
	if err != nil || got.Ahead != 1 || got.Stale != 0 || got.Fixed() != 0 {
		t.Fatalf("rebuild = %+v %v", got, err)
	}
	if row, _ := store.Get(background, "a-task"); row.State != string(Passed) {
		t.Fatalf("the row was set back to %q", row.State)
	}
}

func TestAForcedRebuildWritesARowThatIsAheadOfItsFile(t *testing.T) {
	root, store := restoredFromBackup(t)
	got, err := RebuildWith(background, store, root, RebuildOptions{Force: true}, nil)
	if err != nil || got.Ahead != 0 || got.Stale != 1 {
		t.Fatalf("forced rebuild = %+v %v", got, err)
	}
	if row, _ := store.Get(background, "a-task"); row.State != string(Running) {
		t.Fatalf("the row says %q, and the file says running", row.State)
	}
	if again, err := RebuildWith(background, store, root, RebuildOptions{Force: true}, nil); err != nil || again.Fixed() != 0 || again.Ahead != 0 {
		t.Fatalf("second forced rebuild = %+v %v", again, err)
	}
}

func TestAForcedRebuildStillRemovesWhatIsGone(t *testing.T) {
	root, store := restoredFromBackup(t)
	if err := store.Upsert(background, taskindex.Row{ID: "ghost", State: "passed", Seq: 1}); err != nil {
		t.Fatal(err)
	}
	got, err := RebuildWith(background, store, root, RebuildOptions{Force: true}, nil)
	if err != nil || got.Gone != 1 || got.Stale != 1 {
		t.Fatalf("forced rebuild = %+v %v", got, err)
	}
}
