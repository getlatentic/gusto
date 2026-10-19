package usage_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/spend"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
)

func recPtr[T any](v T) *T { return &v }

func recCall(model string, at int64) spend.Call {
	return spend.Call{
		Path: "/v1/chat/completions", Status: 200, Took: 40, Credential: "platform", AtUnix: at, Model: model,
		InputTokens: recPtr(int64(100)), OutputTokens: recPtr(int64(20)), CachedTokens: recPtr(int64(50)), USD: recPtr(0.5),
	}
}

func recAda(task string) (usage.Who, bool) {
	return usage.Who{Session: "s-" + task, Person: "ada@acme.com", Team: "platform", Engine: "claude-code", Agent: "reviewer"}, true
}

func recNamed(model, credential string) string { return credential + ":" + model }

// recStore is a store that keeps each batch it is handed, and can be made to fail or to wait.
type recStore struct {
	*usagetest.Fake
	mu      sync.Mutex
	batches [][]usage.Row
	fail    func(attempt int) error
	hold    chan struct{}
	tries   int
}

func (c *recStore) Add(ctx context.Context, rows []usage.Row) error {
	if c.hold != nil {
		<-c.hold
	}
	c.mu.Lock()
	c.tries++
	attempt := c.tries
	c.mu.Unlock()
	if c.fail != nil {
		if err := c.fail(attempt); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.batches = append(c.batches, append([]usage.Row(nil), rows...))
	c.mu.Unlock()
	return c.Fake.Add(ctx, rows)
}

func (c *recStore) seen() [][]usage.Row {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.batches
}

func TestACallBecomesARowWithWhoMadeItAndWhoAnswered(t *testing.T) {
	store := &recStore{Fake: usagetest.New()}
	r := usage.NewRecorder(store, recAda, recNamed)
	refused := spend.RefusedCall("/v1/chat/completions", "m1", 1_700_000_100, &spend.Refusal{Cap: "team", Reason: "spent"})
	r.Record("t1", 4, recCall("m1", 1_700_000_000))
	r.Record("t1", 5, refused)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	rows := store.All()
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	for _, row := range rows {
		if row.Task != "t1" || row.Session != "s-t1" || row.Person != "ada@acme.com" || row.Team != "platform" ||
			row.Engine != "claude-code" || row.Agent != "reviewer" || row.Model != "m1" {
			t.Errorf("row %+v", row)
		}
		if row.Line == 4 && (row.Provider != "platform:m1" || row.Credential != "platform" || row.At.Unix() != 1_700_000_000 || *row.InputTokens != 100 || *row.CachedTokens != 50 || *row.USD != 0.5 || row.Status != 200 || row.TookMS != 40) {
			t.Errorf("row %+v", row)
		}
		if row.Line == 5 && (row.Refused != "team" || row.Status != 402) {
			t.Errorf("row %+v", row)
		}
	}
}

func TestACallToAnotherHostIsNotARow(t *testing.T) {
	store := usagetest.New()
	r := usage.NewRecorder(store, recAda, recNamed)
	r.Record("t1", 0, spend.Call{Host: "example.com:443", AtUnix: 1})
	r.Close()
	if n := len(store.All()); n != 0 {
		t.Fatalf("%d rows", n)
	}
}

func TestATaskNobodyKnowsIsRecordedUnderUnknownWithItselfAsTheSession(t *testing.T) {
	store := usagetest.New()
	r := usage.NewRecorder(store, func(string) (usage.Who, bool) { return usage.Who{}, false }, recNamed)
	r.Record("orphan", 0, recCall("m1", 5))
	r.Close()
	rows := store.All()
	if len(rows) != 1 || rows[0].Person != "unknown" || rows[0].Session != "orphan" {
		t.Fatalf("%+v", rows)
	}
}

func TestRowsAreWrittenInBatchesOfAtMostFiveHundredInTheOrderRecorded(t *testing.T) {
	store := &recStore{Fake: usagetest.New()}
	r := usage.NewRecorder(store, recAda, recNamed)
	for i := 0; i < 1200; i++ {
		r.Record("t1", i, recCall("m1", int64(i+1)))
	}
	r.Close()
	next := 0
	for _, batch := range store.seen() {
		if len(batch) > 500 {
			t.Fatalf("a batch of %d", len(batch))
		}
		for _, row := range batch {
			if row.Line != next {
				t.Fatalf("line %d came where %d was due", row.Line, next)
			}
			next++
		}
	}
	if next != 1200 {
		t.Fatalf("%d rows written", next)
	}
}

func TestACallIsWrittenWithinAFewHundredMillisecondsWithoutClose(t *testing.T) {
	store := usagetest.New()
	r := usage.NewRecorder(store, recAda, recNamed)
	defer r.Close()
	r.Record("t1", 0, recCall("m1", 5))
	deadline := time.Now().Add(3 * time.Second)
	for len(store.All()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("nothing was written")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAFullQueueDropsCountsAndNamesTheTaskAndNeverBlocks(t *testing.T) {
	store := &recStore{Fake: usagetest.New(), hold: make(chan struct{})}
	r := usage.NewRecorderSized(store, recAda, recNamed, 2, time.Millisecond)
	stuck := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			r.Record("t1", i, recCall("m1", int64(i+1)))
		}
		close(stuck)
	}()
	select {
	case <-stuck:
	case <-time.After(3 * time.Second):
		t.Fatal("Record waited for the store")
	}
	if r.DroppedCount() == 0 {
		t.Fatal("nothing was counted as dropped")
	}
	select {
	case <-r.Dropped():
	default:
		t.Fatal("no signal that calls were dropped")
	}
	if lost := r.TakeLost(); len(lost) != 1 || lost[0] != "t1" {
		t.Fatalf("lost %v", lost)
	}
	close(store.hold)
	r.Close()
}

func TestAFailedWriteIsRetriedAndTheLogNeverHoldsThePassword(t *testing.T) {
	var logged bytes.Buffer
	was := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	defer slog.SetDefault(was)
	store := &recStore{Fake: usagetest.New(), fail: func(attempt int) error {
		if attempt < 3 {
			return errors.New(`dial postgres://gusto:hunter2@db.internal:5432/usage failed; password=hunter2`)
		}
		return nil
	}}
	r := usage.NewRecorderSized(store, recAda, recNamed, 10, time.Millisecond)
	r.Record("t1", 0, recCall("m1", 5))
	r.Close()
	if len(store.All()) != 1 {
		t.Fatalf("%d rows after the retries", len(store.All()))
	}
	if bytes.Contains(logged.Bytes(), []byte("hunter2")) || !bytes.Contains(logged.Bytes(), []byte("db.internal")) {
		t.Fatalf("the log is %s", logged.String())
	}
}

func TestAWriteThatKeepsFailingAtCloseIsCountedAsLostNotLostSilently(t *testing.T) {
	store := &recStore{Fake: usagetest.New(), fail: func(int) error { return fmt.Errorf("down") }}
	r := usage.NewRecorderSized(store, recAda, recNamed, 10, time.Millisecond)
	r.Record("t1", 0, recCall("m1", 5))
	r.Close()
	if r.DroppedCount() != 1 {
		t.Fatalf("%d dropped", r.DroppedCount())
	}
	if lost := r.TakeLost(); len(lost) != 1 || lost[0] != "t1" {
		t.Fatalf("lost %v", lost)
	}
}

func TestRecordingAfterCloseAndOnANilRecorderIsHarmless(t *testing.T) {
	r := usage.NewRecorder(usagetest.New(), recAda, recNamed)
	r.Close()
	r.Record("t1", 0, recCall("m1", 5))
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	var none *usage.Recorder
	none.Record("t1", 0, recCall("m1", 5))
}

func TestScrubTakesPasswordsOutOfDatabaseErrors(t *testing.T) {
	for _, in := range []string{
		"postgres://u:secret@h/db: refused", "postgresql://u:secret@h:5432/db?sslmode=disable",
		"host=h user=u password=secret dbname=x", "password='se cret' host=h",
	} {
		if out := usage.Scrub(errors.New(in)); bytes.Contains([]byte(out), []byte("secret")) || bytes.Contains([]byte(out), []byte("se cret")) {
			t.Errorf("%q became %q", in, out)
		}
	}
}
