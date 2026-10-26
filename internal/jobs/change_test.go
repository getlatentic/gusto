package jobs

import (
	"fmt"
	"sync"
	"testing"
)

// changes collects what OnChange is told, as the hook is: with the index's lock held, so it only notes.
type changes struct {
	mu   sync.Mutex
	seen []string
}

func (c *changes) note(record Record) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, fmt.Sprintf("%s:%s", record.ID, record.State))
}

func (c *changes) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seen...)
}

func TestEveryChangeToAJobIsToldInTheOrderItWasMade(t *testing.T) {
	index := at(t)
	var told changes
	index.OnChange(told.note)

	first := queued(t, index, "runs to the end")
	second := queued(t, index, "is stopped while it waits")
	if _, _, err := index.Claim(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := index.Stop(second.ID, "ada@acme.example"); err != nil {
		t.Fatal(err)
	}
	if err := index.Finish(first.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}

	want := []string{
		first.ID + ":queued", second.ID + ":queued", first.ID + ":running", second.ID + ":interrupted", first.ID + ":passed",
	}
	if got := told.list(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("told %v, want %v", got, want)
	}
}

func TestWhatWasRunningWhenTheServerStoppedIsRemembered(t *testing.T) {
	index := at(t)
	running := queued(t, index, "was running")
	waiting := queued(t, index, "was waiting")
	if _, _, err := index.Claim(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	recovered := reopened.Recovered()
	if len(recovered) != 1 || recovered[0].ID != running.ID || recovered[0].State != Interrupted {
		t.Fatalf("recovered %+v, want only %s, interrupted", recovered, running.ID)
	}
	if got, _ := reopened.Get(waiting.ID); got.State != Queued {
		t.Fatalf("a job that was only waiting is %q", got.State)
	}
}
