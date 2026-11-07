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

	page := site.ask(t, "GET", "/jobs/amy-waiting", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("code %d", page.Code)
	}
	for _, want := range []string{"amy@acme.test already has 1 running, the most one person may run at once", "Number 1 in line."} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("the job page lacks %q:\n%s", want, page.Body.String())
		}
	}
	record := site.ask(t, "GET", "/jobs", nil).Body.String()
	if !strings.Contains(record, "Asked by amy@acme.test. amy@acme.test already has 1 running") || !strings.Contains(record, "Number 1 in line.") {
		t.Errorf("the record does not say who is waiting:\n%s", record)
	}
	if running := site.ask(t, "GET", "/jobs/amy-running", nil).Body.String(); strings.Contains(running, "in line") {
		t.Error("a running job was said to be in line")
	}
}
