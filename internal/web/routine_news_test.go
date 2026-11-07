package web

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/notify"
	"github.com/getlatentic/gusto/pkg/task"
)

// waiting makes a routine's turns wait until the returned function lets them end.
func waiting(t *testing.T) (started chan string, release func()) {
	t.Helper()
	started, gate := make(chan string, 4), make(chan struct{})
	testWork = func(ctx context.Context, dir task.Dir) (int, error) {
		started <- dir.Path()
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return 0, nil
	}
	t.Cleanup(func() { testWork = nil })
	closed := false
	return started, func() {
		if !closed {
			closed = true
			close(gate)
		}
	}
}

func ended(t *testing.T, site *site, task string) {
	t.Helper()
	for range 100 {
		if record, _ := site.index.Get(task); record.State.Done() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the turn %s did not end", task)
}

func routinesOf(t *testing.T, site *site, id string) []instance.Routine {
	t.Helper()
	routines, err := site.server.instances.Routines(id)
	if err != nil {
		t.Fatal(err)
	}
	return routines
}

// A slot skipped while the last run is still answering keeps that run's news, which is told when it ends.
// spec/Routines.before-owe.cfg is the trace.
func TestARunsNewsIsToldAfterASlotItWasStillAnsweringIn(t *testing.T) {
	started, release := waiting(t)
	site := open(t)
	t.Cleanup(release)
	site.signIn(t)
	mail := &news{channel: "mail"}
	site.server.UseRoutineNews(notify.Every{mail}, "")
	id, _ := aRoutine(t, site, url.Values{"title": {"Slow"}, "prompt": {"go"}, "every": {"1"}})
	site.server.StartDueRoutines(time.Now())
	<-started
	run := routinesOf(t, site, id)[0].Last.Task

	site.server.StartDueRoutines(time.Now().Add(2 * time.Hour))
	release()
	ended(t, site, run)
	site.server.tellRoutines()
	if len(mail.told) != 1 || mail.told[0].Task != run {
		t.Errorf("the run's news was not told after the skip: %+v", mail.told)
	}
}

// A run that ends and is followed by the next slot's run before the news goes out is still told, and so is the next.
func TestARunsNewsIsToldAfterTheNextRunStarts(t *testing.T) {
	site := open(t)
	site.signIn(t)
	mail := &news{channel: "mail"}
	site.server.UseRoutineNews(notify.Every{mail}, "")
	id, _ := aFinishedRun(t, site)
	first := routinesOf(t, site, id)[0].Last.Task

	site.server.StartDueRoutines(time.Now().Add(2 * time.Hour))
	<-site.ran
	second := routinesOf(t, site, id)[0].Last.Task
	ended(t, site, second)
	site.server.tellRoutines()
	if len(mail.told) != 2 || mail.told[0].Task != first || mail.told[1].Task != second {
		t.Fatalf("told %+v, not %s then %s", mail.told, first, second)
	}
	if owed := routinesOf(t, site, id)[0].Owed; len(owed) != 0 {
		t.Errorf("settled news is still owed: %+v", owed)
	}
}

// A damaged routines.json is said on the agent's page, not shown as an agent with no routines.
// spec/Routines.before-surface.cfg is the trace.
func TestADamagedRoutinesFileIsSaidOnThePage(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, where := aRoutine(t, site, url.Values{"title": {"Morning"}, "prompt": {"go"}, "every": {"1"}})
	if err := os.WriteFile(filepath.Join(site.server.instances.Dir(id), "routines.json"), []byte(`[{"id":"r1",`), 0o644); err != nil {
		t.Fatal(err)
	}
	body := site.ask(t, "GET", where, nil).Body.String()
	if !strings.Contains(body, "routines.json") {
		t.Error("the page does not say its routines cannot be read")
	}
}
