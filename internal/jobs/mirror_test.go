package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/taskindex/taskindextest"
)

// recorder is a Mirror that keeps what it was given, in order, and can be made to fail or to wait.
type recorder struct {
	mu    sync.Mutex
	log   []string
	rows  map[string]taskindex.Row
	fail  error
	gate  chan struct{}
	calls int
}

func newRecorder() *recorder { return &recorder{rows: map[string]taskindex.Row{}} }

func (m *recorder) enter() error {
	m.mu.Lock()
	gate, fail := m.gate, m.fail
	m.calls++
	m.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return fail
}

func (m *recorder) Upsert(_ context.Context, row taskindex.Row) error {
	if err := m.enter(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.log = append(m.log, "upsert "+row.ID+" "+row.State)
	m.rows[row.ID] = row
	return nil
}

func (m *recorder) Delete(_ context.Context, id string) error {
	if err := m.enter(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.log = append(m.log, "delete "+id)
	delete(m.rows, id)
	return nil
}

func (m *recorder) seen() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.log...)
}

func (m *recorder) setFail(err error) {
	m.mu.Lock()
	m.fail = err
	m.mu.Unlock()
}

// settle waits until the mirror has been given every change made so far.
func settle(t *testing.T, index *Index) {
	t.Helper()
	index.guard.Lock()
	p := index.pump
	index.guard.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.flush(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestEveryWriteOfAStateFileReachesTheMirrorInOrder(t *testing.T) {
	index := at(t)
	mirror := newRecorder()
	index.UseMirror(mirror)
	a, b := queued(t, index, "first"), queued(t, index, "second")
	if _, found, err := index.Claim(); err != nil || !found {
		t.Fatalf("claim: %v %v", found, err)
	}
	if err := index.Finish(a.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := index.Stop(b.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	if err := index.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	settle(t, index)
	want := []string{"upsert " + a.ID + " queued", "upsert " + b.ID + " queued", "upsert " + a.ID + " running",
		"upsert " + a.ID + " passed", "upsert " + b.ID + " interrupted", "delete " + a.ID}
	if got := mirror.seen(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("the mirror saw\n%v\nwant\n%v", got, want)
	}
	got := mirror.rows[b.ID]
	if got.StoppedBy != "ada" || got.Exit != -1 || got.Ended == 0 {
		t.Fatalf("the row of a stopped task: %+v", got)
	}
}

func TestAStateFileThatCouldNotBeWrittenIsNotMirrored(t *testing.T) {
	index := at(t)
	mirror := newRecorder()
	index.UseMirror(mirror)
	record := Record{ID: "no-such-directory", State: Queued, Brief: "x", Queued: Now()}
	if err := index.Add(record); err == nil {
		t.Fatal("a task whose directory is missing was added")
	}
	settle(t, index)
	if got := mirror.seen(); len(got) != 0 {
		t.Fatalf("the mirror saw %v", got)
	}
}

func TestAFailingMirrorNeverStopsATaskAndReconcileMendsItAfterwards(t *testing.T) {
	index := at(t)
	mirror := newRecorder()
	mirror.setFail(errors.New("connection refused"))
	index.UseMirror(mirror)
	a := queued(t, index, "first")
	if _, found, err := index.Claim(); err != nil || !found {
		t.Fatalf("claim failed while the mirror was down: %v %v", found, err)
	}
	if err := index.Finish(a.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	settle(t, index)
	if stats := index.MirrorStats(); stats.Failed != 3 || stats.Dropped != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	select {
	case <-index.MirrorTrouble():
	default:
		t.Fatal("no signal that the mirror needs mending")
	}
	if got, _ := index.Get(a.ID); got.State != Passed {
		t.Fatalf("the task is %s", got.State)
	}
	mirror.setFail(nil)
	store := taskindextest.New()
	index.UseMirror(store)
	result, err := index.Reconcile(context.Background(), store, nil)
	if err != nil || result.Missing != 1 {
		t.Fatalf("reconcile = %+v %v", result, err)
	}
	if row, err := store.Get(context.Background(), a.ID); err != nil || row.State != "passed" {
		t.Fatalf("row = %+v %v", row, err)
	}
}

func TestASlowMirrorDoesNotHoldUpTheClaimPath(t *testing.T) {
	index := at(t)
	mirror := newRecorder()
	mirror.gate = make(chan struct{})
	index.useMirror(mirror, 4)
	started := time.Now()
	for n := 0; n < 30; n++ {
		queued(t, index, fmt.Sprintf("task %d", n))
	}
	for n := 0; n < 30; n++ {
		if _, found, err := index.Claim(); err != nil || !found {
			t.Fatalf("claim %d: %v %v", n, found, err)
		}
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Fatalf("60 changes took %v behind a mirror that was waiting", took)
	}
	if stats := index.MirrorStats(); stats.Dropped == 0 {
		t.Fatalf("nothing was dropped behind a full queue: %+v", stats)
	}
	close(mirror.gate)
	store := taskindextest.New()
	index.UseMirror(store)
	if _, err := index.Reconcile(context.Background(), store, nil); err != nil {
		t.Fatal(err)
	}
	sameAsMemory(t, index, store)
}

// sameAsMemory is true when the store holds, for every task the index holds, exactly its row.
func sameAsMemory(t *testing.T, index *Index, store *taskindextest.Fake) {
	t.Helper()
	rows := map[string]taskindex.Row{}
	for _, row := range store.All() {
		rows[row.ID] = row
	}
	records := index.List()
	if len(rows) != len(records) {
		t.Fatalf("the store holds %d rows for %d tasks", len(rows), len(records))
	}
	for _, record := range records {
		if rows[record.ID] != RowOf(record) {
			t.Errorf("row of %s is\n%+v\nwant\n%+v", record.ID, rows[record.ID], RowOf(record))
		}
	}
}
