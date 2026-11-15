package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/tasks"
)

// waitingTask leaves a task that is asked for by who, in the state given.
func waitingTask(t *testing.T, site *site, id, who string, state tasks.State, seq int64) {
	t.Helper()
	spentTask(t, site.index.Root(), id, "", priced)
	if err := site.index.Add(tasks.Record{ID: id, State: state, Brief: "brief " + id, Harness: "fixture", Asked: who, Seq: seq, Queued: tasks.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestAWaitingTaskSaysWhyOnItsPageAndInTheRecord(t *testing.T) {
	site := open(t)
	site.server.index.SetLimits(tasks.Limits{Person: 1})
	waitingTask(t, site, "amy-running", "amy@acme.test", tasks.Running, 1)
	waitingTask(t, site, "amy-waiting", "amy@acme.test", tasks.Queued, 2)
	site.signIn(t)

	if page := site.ask(t, "GET", "/jobs/amy-waiting", nil); page.Code != http.StatusOK {
		t.Fatalf("code %d", page.Code)
	}
	waiting := site.task(t, "amy-waiting").Waiting
	for _, want := range []string{"amy@acme.test already has 1 running, the most one person may run at once", "Number 1 in line."} {
		if !strings.Contains(waiting, want) {
			t.Errorf("the job page lacks %q: %q", want, waiting)
		}
	}
	// The record says "Asked by" before why it waits (web/src/pages/work/activity-page.test.tsx).
	if listed := site.activity(t, "").run(t, "amy-waiting"); listed.AskedBy != "amy@acme.test" ||
		!strings.HasPrefix(listed.Waiting, "amy@acme.test already has 1 running") || !strings.Contains(listed.Waiting, "Number 1 in line.") {
		t.Errorf("the record does not say who is waiting: %+v", listed)
	}
	if running := site.task(t, "amy-running"); strings.Contains(running.Waiting, "in line") {
		t.Error("a running job was said to be in line")
	}
	if listed := site.activity(t, "").run(t, "amy-running"); listed.Waiting != "" || listed.AskedBy != "" {
		t.Errorf("a running job was said to be in line in the record: %+v", listed)
	}
}
