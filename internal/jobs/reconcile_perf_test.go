package jobs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
	"github.com/getlatentic/gusto/internal/taskindex"
)

// bigEnv turns on the test that makes 100,000 task directories, which takes a while to make.
const bigEnv = "GUSTO_TEST_BIG"

func TestReconcileOnAHundredThousandTaskDirectories(t *testing.T) {
	if os.Getenv(bigEnv) == "" {
		t.Skip(bigEnv + " is not set")
	}
	const tasks = 100_000
	root := t.TempDir()
	started := time.Now()
	for i := 0; i < tasks; i++ {
		record := Record{ID: fmt.Sprintf("2026%04d-%04d-task-%d", 1+i/9000, i%2400, i), State: []State{Passed, Passed, Failed, Interrupted}[i%4],
			Brief: "fix the thing in module " + fmt.Sprint(i%997), Harness: "codex", Asked: fmt.Sprintf("person-%02d@acme.com", i%60),
			Repo: "acme/web", Seq: int64(i + 1), Queued: int64(1_790_000_000 + i), Ended: int64(1_790_000_100 + i)}
		dir := filepath.Join(root, record.ID)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := write(record.Dir(root), record); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("made %d task directories in %v", tasks, time.Since(started).Round(time.Millisecond))
	started = time.Now()
	index, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("jobs.Open read them in %v", time.Since(started).Round(time.Millisecond))
	started = time.Now()
	if _, err := Scan(root); err != nil {
		t.Fatal(err)
	}
	t.Logf("jobs.Scan read them in %v", time.Since(started).Round(time.Millisecond))
	t.Run("sqlite", func(t *testing.T) {
		store, err := taskindex.Open(background, "", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		reconcileTimes(t, index, store)
	})
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		store, err := taskindex.Open(background, dbkittest.PostgresURL(t), "")
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		reconcileTimes(t, index, store)
	})
}

func reconcileTimes(t *testing.T, index *Index, store taskindex.Store) {
	t.Helper()
	index.UseMirror(store)
	defer index.CloseMirror(background)
	for _, step := range []string{"into an empty index", "against a current index"} {
		started := time.Now()
		result, err := index.Reconcile(background, store, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Reconcile %s: %v, %+v", step, time.Since(started).Round(time.Millisecond), result)
	}
	for _, drift := range []int{10, 1000} {
		rows, _, err := store.List(background, taskindex.Filter{}, taskindex.Page{Limit: drift})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if err := store.Delete(background, row.ID); err != nil {
				t.Fatal(err)
			}
		}
		started := time.Now()
		result, err := index.Reconcile(background, store, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Reconcile with %d rows missing: %v, %+v", drift, time.Since(started).Round(time.Millisecond), result)
	}
}
