package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/taskindex/taskindextest"
)

var background = context.Background()

func reconciled(t *testing.T, index *Index, store taskindex.Store) ReconcileResult {
	t.Helper()
	result, err := index.Reconcile(background, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestReconcileFillsAnEmptyIndexAndThenFindsNothingToDo(t *testing.T) {
	index := at(t)
	for n := 0; n < 5; n++ {
		queued(t, index, fmt.Sprintf("task %d", n))
	}
	store := taskindextest.New()
	if got := reconciled(t, index, store); got.Tasks != 5 || got.Missing != 5 || got.Stale != 0 || got.Gone != 0 {
		t.Fatalf("first run = %+v", got)
	}
	sameAsMemory(t, index, store)
	if got := reconciled(t, index, store); got.Fixed() != 0 {
		t.Fatalf("second run = %+v", got)
	}
}

func TestReconcileMendsEveryKindOfDrift(t *testing.T) {
	index := at(t)
	missing, stale, quiet, gone := queued(t, index, "missing"), queued(t, index, "stale"), queued(t, index, "stale in a field"), queued(t, index, "gone")
	store := taskindextest.New()
	reconciled(t, index, store)
	for _, id := range []string{stale.ID, quiet.ID, gone.ID} {
		if _, found, err := index.Claim(); err != nil || !found {
			t.Fatal(found, err)
		}
		_ = id
	}
	if err := index.Finish(stale.ID, Failed, 3); err != nil {
		t.Fatal(err)
	}
	if _, _, err := index.Stop(quiet.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := index.Finish(gone.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(background, missing.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(background, taskindex.Row{ID: "20200101-removed", State: "passed", Queued: 1, Seq: 1}); err != nil {
		t.Fatal(err)
	}
	if err := index.Remove(gone.ID); err != nil {
		t.Fatal(err)
	}
	if got := reconciled(t, index, store); got != (ReconcileResult{Tasks: 3, Missing: 1, Stale: 2, Gone: 2}) {
		t.Fatalf("run = %+v", got)
	}
	sameAsMemory(t, index, store)
	if _, err := store.Get(background, "20200101-removed"); err == nil {
		t.Fatal("the row of a task that is gone was kept")
	}
}

func TestReconcileTakesATaskThatIsOnlyOnDiskAndLeavesOneWhoseStateIsBeingWritten(t *testing.T) {
	index := at(t)
	queued(t, index, "known")
	onlyOnDisk := Record{ID: "20260101-0000-written-elsewhere", State: Passed, Brief: "elsewhere", Asked: "cy", Queued: 5, Seq: 9}
	if err := os.MkdirAll(onlyOnDisk.Dir(index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := write(onlyOnDisk.Dir(index.Root()), onlyOnDisk); err != nil {
		t.Fatal(err)
	}
	store := taskindextest.New()
	partial := Record{ID: "20260101-0001-half-written"}
	if err := os.MkdirAll(partial.Dir(index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial.Dir(index.Root()).Path(), "state.json"), []byte(`{"id": "20260101`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(background, RowOf(partial)); err != nil {
		t.Fatal(err)
	}
	if got := reconciled(t, index, store); got.Tasks != 2 || got.Missing != 2 || got.Gone != 1 {
		t.Fatalf("run = %+v", got)
	}
	if row, err := store.Get(background, onlyOnDisk.ID); err != nil || row != RowOf(onlyOnDisk) {
		t.Fatalf("row = %+v %v", row, err)
	}
	if _, err := store.Get(background, partial.ID); err != nil {
		t.Fatalf("the row of a task whose state.json could not be read just then was removed: %v", err)
	}
}

func TestReconcileReportsItsProgress(t *testing.T) {
	index := at(t)
	for n := 0; n < 3; n++ {
		queued(t, index, fmt.Sprintf("task %d", n))
	}
	var seen []taskindex.Status
	progress := &taskindex.Progress{Changed: func(s taskindex.Status) { seen = append(seen, s) }}
	if _, err := index.Reconcile(background, taskindextest.New(), progress); err != nil {
		t.Fatal(err)
	}
	last := seen[len(seen)-1]
	if last.Building || last.Done != 3 || last.Total != 3 {
		t.Fatalf("the last status is %+v of %+v", last, seen)
	}
	if first := seen[1]; !first.Building || first.Total != 3 || first.Done != 0 {
		t.Fatalf("the status when it began is %+v of %+v", first, seen)
	}
}

// slowStore is a store that takes its time, so that tasks change between a reconcile's steps.
type slowStore struct{ *taskindextest.Fake }

func (s slowStore) Upsert(ctx context.Context, row taskindex.Row) error {
	time.Sleep(200 * time.Microsecond)
	return s.Fake.Upsert(ctx, row)
}

func (s slowStore) UpsertMany(ctx context.Context, rows []taskindex.Row) error {
	time.Sleep(time.Millisecond)
	return s.Fake.UpsertMany(ctx, rows)
}

func (s slowStore) Delete(ctx context.Context, id string) error {
	time.Sleep(200 * time.Microsecond)
	return s.Fake.Delete(ctx, id)
}

func (s slowStore) Indexed(ctx context.Context) (map[string]taskindex.Entry, error) {
	time.Sleep(5 * time.Millisecond)
	return s.Fake.Indexed(ctx)
}

func TestTasksThatChangeWhileReconcileRunsAreNeverSetBack(t *testing.T) {
	index := at(t)
	store := slowStore{taskindextest.New()}
	index.UseMirror(store)
	var started, finished sync.WaitGroup
	stop := make(chan struct{})
	finished.Add(1)
	go func() {
		defer finished.Done()
		for {
			select {
			case <-stop:
				return
			default:
				if _, err := index.Reconcile(background, store, nil); err != nil {
					t.Error(err)
					return
				}
			}
		}
	}()
	for worker := 0; worker < 4; worker++ {
		started.Add(1)
		go func() {
			defer started.Done()
			for n := 0; n < 25; n++ {
				id, err := index.Reserve(fmt.Sprintf("worker %d task %d", worker, n), time.Now())
				if err != nil {
					t.Error(err)
					return
				}
				record := Record{ID: id, State: Queued, Brief: "b", Harness: "fixture", Asked: "a", Queued: Now()}
				if err := index.Add(record); err != nil {
					t.Error(err)
					return
				}
				if _, _, err := index.Claim(); err != nil {
					t.Error(err)
					return
				}
				switch n % 5 {
				case 0:
					_ = index.Finish(id, Passed, 0)
					_ = index.Remove(id)
				case 1:
					_ = index.Finish(id, Failed, 1)
				}
			}
		}()
	}
	started.Wait()
	close(stop)
	finished.Wait()
	settle(t, index)
	if got := reconciled(t, index, store); got.Fixed() != 0 {
		t.Fatalf("after the changes stopped, a reconcile still had %+v to fix", got)
	}
	sameAsMemory(t, index, store.Fake)
}

func TestRebuildReadsOnlyTheFilesAndNumbersOldTasksAsAServerDoes(t *testing.T) {
	root := t.TempDir()
	old := []Record{{ID: "a-old", State: Passed, Queued: 10, Brief: "x"}, {ID: "b-old", State: Failed, Queued: 10, Brief: "y"}, {ID: "c-new", State: Queued, Queued: 20, Seq: 4, Brief: "z"}}
	for _, record := range old {
		if err := os.MkdirAll(record.Dir(root).Path(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := write(record.Dir(root), record); err != nil {
			t.Fatal(err)
		}
	}
	store := taskindextest.New()
	if got, err := Rebuild(background, store, root, false, nil); err != nil || got.Missing != 3 {
		t.Fatalf("rebuild = %+v %v", got, err)
	}
	served, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range served.List() {
		row, err := store.Get(background, record.ID)
		if err != nil || row.Seq != record.Seq {
			t.Errorf("%s: row seq %d, server seq %d (%v)", record.ID, row.Seq, record.Seq, err)
		}
	}
	if err := store.Upsert(background, taskindex.Row{ID: "ghost", State: "passed", Seq: 1}); err != nil {
		t.Fatal(err)
	}
	if got, err := Rebuild(background, store, root, false, nil); err != nil || got.Gone != 1 || got.Fixed() != 1 {
		t.Fatalf("second rebuild = %+v %v", got, err)
	}
	if got, err := Rebuild(background, store, root, true, nil); err != nil || got.Missing != 3 || got.Gone != 0 {
		t.Fatalf("rebuild from scratch = %+v %v", got, err)
	}
}
