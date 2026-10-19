package usage_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/spend"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
)

// bfRecord writes a task's proxy record, one line to each call, and a string as it is.
func bfRecord(t *testing.T, jobs, task string, lines ...any) string {
	t.Helper()
	dir := filepath.Join(jobs, task, "out")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, line := range lines {
		if s, ok := line.(string); ok {
			text.WriteString(s)
			continue
		}
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		text.Write(raw)
		text.WriteString("\n")
	}
	path := filepath.Join(dir, "proxy.jsonl")
	if err := os.WriteFile(path, []byte(text.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func bfByKey(rows []usage.Row) map[string]usage.Row {
	out := map[string]usage.Row{}
	for _, r := range rows {
		out[r.Task+"/"+string(rune('0'+r.Line))] = r
	}
	return out
}

func bfRun(t *testing.T, store usage.Store, jobs string) usage.Result {
	t.Helper()
	got, err := usage.Backfill(context.Background(), store, jobs, recAda, recNamed, nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestBackfillAddsEveryModelCallAndSkipsOtherHosts(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "a", recCall("m1", 10), spend.Call{Host: "x.com:443", AtUnix: 11}, recCall("m2", 12))
	bfRecord(t, jobs, "b", recCall("m1", 20))
	got := bfRun(t, store, jobs)
	if got.Tasks != 2 || got.Rows != 3 || len(store.All()) != 3 {
		t.Fatalf("%+v, %d rows", got, len(store.All()))
	}
	lines := map[string][]int{}
	for _, r := range store.All() {
		lines[r.Task] = append(lines[r.Task], r.Line)
	}
	sort.Ints(lines["a"])
	if !reflect.DeepEqual(lines["a"], []int{0, 2}) {
		t.Fatalf("task a held lines %v", lines["a"])
	}
}

func TestBackfillTwiceAddsNothingTheSecondTime(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "a", recCall("m1", 10), recCall("m1", 11))
	bfRun(t, store, jobs)
	if again := bfRun(t, store, jobs); again.Rows != 0 || len(store.All()) != 2 {
		t.Fatalf("%+v, %d rows", again, len(store.All()))
	}
}

func TestBackfillReadsOnlyTheLinesPastWhatTheIndexHolds(t *testing.T) {
	jobs, store := t.TempDir(), &recStore{Fake: usagetest.New()}
	bfRecord(t, jobs, "a", recCall("m1", 10), recCall("m1", 11), recCall("m1", 12), recCall("m1", 13))
	store.Fake.Add(context.Background(), []usage.Row{{Task: "a", Line: 0}, {Task: "a", Line: 1}})
	bfRecord(t, jobs, "held", recCall("m1", 10))
	store.Fake.Add(context.Background(), []usage.Row{{Task: "held", Line: 0}})
	got := bfRun(t, store, jobs)
	if got.Rows != 2 {
		t.Fatalf("%+v", got)
	}
	for _, batch := range store.seen() {
		for _, r := range batch {
			if r.Task == "held" || r.Line < 2 {
				t.Fatalf("a held line was read again: %+v", r)
			}
		}
	}
}

func TestBackfillPassesOverLinesThatAreNotCallsAndSaysHowMany(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "a", recCall("m1", 10), "{not json\n", "\n", recCall("m1", 12))
	got := bfRun(t, store, jobs)
	if got.Rows != 2 || got.Malformed != 1 {
		t.Fatalf("%+v", got)
	}
	if rows := bfByKey(store.All()); rows["a/3"].Model != "m1" {
		t.Fatalf("the line after the bad ones kept its place: %+v", store.All())
	}
}

func TestALineStillBeingWrittenIsLeftForTheNextRun(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	whole, _ := json.Marshal(recCall("m1", 11))
	path := bfRecord(t, jobs, "a", recCall("m1", 10), string(whole[:len(whole)/2]))
	if got := bfRun(t, store, jobs); got.Rows != 1 || got.Malformed != 0 {
		t.Fatalf("%+v", got)
	}
	if err := os.WriteFile(path, []byte(string(mustJSON(t, recCall("m1", 10)))+"\n"+string(whole)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := bfRun(t, store, jobs); got.Rows != 1 || len(store.All()) != 2 {
		t.Fatalf("%+v, %d rows", got, len(store.All()))
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// bfInterrupting cancels the run after it has been handed a number of batches.
type bfInterrupting struct {
	*usagetest.Fake
	after  int
	cancel context.CancelFunc
	mu     sync.Mutex
	n      int
}

func (i *bfInterrupting) Add(ctx context.Context, rows []usage.Row) error {
	i.mu.Lock()
	i.n++
	stop := i.n >= i.after
	i.mu.Unlock()
	err := i.Fake.Add(ctx, rows)
	if stop {
		i.cancel()
	}
	return err
}

func TestAnInterruptedRunResumesAndEndsWithEveryRowOnce(t *testing.T) {
	jobs := t.TempDir()
	for _, task := range []string{"a", "b", "c", "d"} {
		bfRecord(t, jobs, task, recCall("m1", 10), recCall("m1", 11), recCall("m1", 12))
	}
	ctx, cancel := context.WithCancel(context.Background())
	store := &bfInterrupting{Fake: usagetest.New(), after: 2, cancel: cancel}
	if _, err := usage.Backfill(ctx, store, jobs, recAda, recNamed, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("the interrupted run ended with %v", err)
	}
	if n := len(store.All()); n == 0 || n >= 12 {
		t.Fatalf("%d rows after the interruption", n)
	}
	bfRun(t, store.Fake, jobs)
	if n := len(store.All()); n != 12 {
		t.Fatalf("%d rows after resuming", n)
	}
}

func TestProgressCountsTasksAndEndsDone(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "a", recCall("m1", 10))
	bfRecord(t, jobs, "b", recCall("m1", 10))
	var seen []usage.Status
	progress := &usage.Progress{Changed: func(s usage.Status) { seen = append(seen, s) }}
	if _, err := usage.Backfill(context.Background(), store, jobs, recAda, recNamed, progress); err != nil {
		t.Fatal(err)
	}
	if seen[0] != (usage.Status{Building: true, Total: 2}) {
		t.Fatalf("it began as %+v", seen[0])
	}
	if end := progress.Status(); end.Building || end.Done != 2 || end.Total != 2 {
		t.Fatalf("it ended as %+v", end)
	}
	var none *usage.Progress
	if none.Status() != (usage.Status{}) {
		t.Fatal("a rebuild nobody watches has a status")
	}
}

func TestBackfillAndTheRecorderMakeTheSameRows(t *testing.T) {
	jobs := t.TempDir()
	calls := []spend.Call{
		recCall("m1", 10), {Host: "x:443", AtUnix: 11},
		spend.RefusedCall("/v1/x", "m2", 12, &spend.Refusal{Cap: "job", Reason: "spent"}),
		{Path: "/v1/x", Model: "m3", Credential: "k", AtUnix: 13, Status: 200, Translated: "messages"},
	}
	lines := make([]any, len(calls))
	for i, c := range calls {
		lines[i] = c
	}
	bfRecord(t, jobs, "a", lines...)
	built, live := usagetest.New(), usagetest.New()
	bfRun(t, built, jobs)
	r := usage.NewRecorder(live, recAda, recNamed)
	for i, c := range calls {
		r.Record("a", i, c)
	}
	r.Close()
	if !reflect.DeepEqual(bfByKey(built.All()), bfByKey(live.All())) || len(built.All()) != 3 {
		t.Fatalf("\nrebuilt %+v\nlive    %+v", built.All(), live.All())
	}
}

func TestATaskNobodyKnowsIsIndexedUnderUnknown(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "orphan", recCall("m1", 10))
	if _, err := usage.Backfill(context.Background(), store, jobs, func(string) (usage.Who, bool) { return usage.Who{}, false }, recNamed, nil); err != nil {
		t.Fatal(err)
	}
	if rows := store.All(); len(rows) != 1 || rows[0].Person != "unknown" || rows[0].Session != "orphan" {
		t.Fatalf("%+v", rows)
	}
}

func TestRepairReadsATaskFromItsFirstLineToFillAGap(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "a", recCall("m1", 10), recCall("m1", 11), recCall("m1", 12))
	store.Add(context.Background(), []usage.Row{{Task: "a", Line: 2}})
	if got := bfRun(t, store, jobs); got.Rows != 0 {
		t.Fatalf("a backfill looked behind the last line held: %+v", got)
	}
	if _, err := usage.Repair(context.Background(), store, jobs, recAda, recNamed, nil, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if n := len(store.All()); n != 3 {
		t.Fatalf("%d rows after the repair", n)
	}
}

func TestKeepFillsTheIndexThenRepairsWhatTheRecorderLost(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "a", recCall("m1", 10), recCall("m1", 11))
	rec := usage.NewRecorder(store, recAda, recNamed)
	defer rec.Close()
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { usage.Keep(ctx, store, jobs, recAda, recNamed, rec, nil); close(done) }()
	waitForRows(t, store, 2)
	bfRecord(t, jobs, "a", recCall("m1", 10), recCall("m1", 11), recCall("m1", 12))
	usage.Lose(rec, "a")
	waitForRows(t, store, 3)
	stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Keep did not end with its context")
	}
}

func waitForRows(t *testing.T, store *usagetest.Fake, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(store.All()) < want {
		if time.Now().After(deadline) {
			t.Fatalf("%d rows, wanted %d", len(store.All()), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestADeletedSQLiteIndexIsRebuiltToTheSameRows(t *testing.T) {
	jobs, data := t.TempDir(), t.TempDir()
	bfRecord(t, jobs, "a", recCall("m1", 10), spend.Call{Host: "x:443", AtUnix: 11}, recCall("m2", 12))
	bfRecord(t, jobs, "b", recCall("m1", 20), "garbage\n", recCall("m1", 21))
	read := func() []usage.Row {
		store, err := usage.Open(context.Background(), "", data)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if _, err := usage.Backfill(context.Background(), store, jobs, recAda, recNamed, nil); err != nil {
			t.Fatal(err)
		}
		var rows []usage.Row
		for _, session := range []string{"s-a", "s-b"} {
			got, _, err := store.Calls(context.Background(), session, usage.Filter{}, usage.Page{Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			rows = append(rows, got...)
		}
		return rows
	}
	first := read()
	if len(first) != 4 {
		t.Fatalf("%d rows", len(first))
	}
	for _, name := range []string{"usage.db", "usage.db-wal", "usage.db-shm"} {
		os.Remove(filepath.Join(data, name))
	}
	if second := read(); !reflect.DeepEqual(first, second) {
		t.Fatalf("\nfirst  %+v\nsecond %+v", first, second)
	}
}

func TestARebuildNeitherDeletesNorBringsBackWhatRetentionRemoved(t *testing.T) {
	jobs, store := t.TempDir(), usagetest.New()
	bfRecord(t, jobs, "gone", recCall("m1", 10))
	bfRecord(t, jobs, "kept", recCall("m1", 11))
	bfRun(t, store, jobs)
	if err := os.RemoveAll(filepath.Join(jobs, "gone")); err != nil {
		t.Fatal(err)
	}
	bfRun(t, store, jobs)
	tasks := map[string]bool{}
	for _, r := range store.All() {
		tasks[r.Task] = true
	}
	if !tasks["gone"] || !tasks["kept"] || len(store.All()) != 2 {
		t.Fatalf("%+v", store.All())
	}
}

func bfRowsOf(t *testing.T, store usage.Store) []usage.Row {
	t.Helper()
	var rows []usage.Row
	for _, session := range []string{"s-a", "s-b"} {
		got, _, err := store.Calls(context.Background(), session, usage.Filter{}, usage.Page{Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, got...)
	}
	return rows
}

// Postgres is run when GUSTO_TEST_POSTGRES_URL names a server, in a schema of its own.
func TestAnIndexIsEmptiedAndRebuiltFromScratchToTheSameRows(t *testing.T) {
	stores := map[string]func(testing.TB) usage.Store{"sqlite": openSQLite}
	if os.Getenv(postgresEnv) != "" {
		stores["postgres"] = openPostgres
	}
	for name, open := range stores {
		t.Run(name, func(t *testing.T) {
			jobs := t.TempDir()
			bfRecord(t, jobs, "a", recCall("m1", 10), spend.Call{Host: "x:443", AtUnix: 11}, recCall("m2", 12))
			bfRecord(t, jobs, "b", recCall("m1", 20), "garbage\n", recCall("m1", 21))
			store := open(t)
			bfRun(t, store, jobs)
			first := bfRowsOf(t, store)
			if len(first) != 4 {
				t.Fatalf("%d rows", len(first))
			}
			removed, err := store.Reset(context.Background())
			if err != nil || removed != 4 {
				t.Fatalf("removed %d (%v)", removed, err)
			}
			if held, _ := store.Indexed(context.Background()); len(held) != 0 {
				t.Fatalf("still holds %v", held)
			}
			if got := bfRun(t, store, jobs); got.Rows != 4 {
				t.Fatalf("%+v", got)
			}
			if second := bfRowsOf(t, store); !reflect.DeepEqual(first, second) {
				t.Fatalf("\nfirst  %+v\nsecond %+v", first, second)
			}
		})
	}
}
