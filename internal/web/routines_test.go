package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/job"
)

// aRoutine adds a routine through the form and says which instance it is on.
func aRoutine(t *testing.T, site *site, form url.Values) (instanceID string, where string) {
	t.Helper()
	where = startedInstance(t, site)
	answer := site.ask(t, "POST", where+"/routines", form)
	if answer.Code != http.StatusSeeOther || strings.Contains(answer.Header().Get("Location"), "refused=") {
		t.Fatalf("the routine was not taken: %d to %q", answer.Code, answer.Header().Get("Location"))
	}
	return strings.TrimPrefix(where, "/instances/"), where
}

func TestADueRoutineStartsATurnInItsOwnConversationOnceAPerSlot(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, where := aRoutine(t, site, url.Values{"title": {"Morning"}, "prompt": {"what changed?"}, "every": {"6"}})
	routine := site.server.instances.Routines(id)[0]

	site.server.StartDueRoutines(time.Now())
	thread, _ := site.server.instances.Thread(id, routine.Thread)
	if len(thread.Turns) != 1 {
		t.Fatalf("a routine never run is due, and its conversation holds %d turns", len(thread.Turns))
	}
	record, _ := site.index.Get(thread.Turns[0])
	if !strings.HasPrefix(record.Brief, "[routine] Morning: what changed?") || record.Instance != id || record.Thread != routine.Thread {
		t.Errorf("the turn is %+v", record)
	}
	if got := site.server.instances.Routines(id)[0].Last; got == nil || got.Job != record.ID {
		t.Errorf("the slot kept %+v", got)
	}

	site.server.StartDueRoutines(time.Now())
	if again, _ := site.server.instances.Thread(id, routine.Thread); len(again.Turns) != 1 {
		t.Errorf("six hours had not passed, and a second turn started: %d", len(again.Turns))
	}
	if body := site.ask(t, "GET", where, nil).Body.String(); !strings.Contains(body, "Morning") || !strings.Contains(body, "every 6 hours") {
		t.Error("the agent's page does not list its routine and when it runs")
	}
}

func TestARoutineWhoseLastRunIsStillAnsweringSkipsTheSlotAndSaysWhy(t *testing.T) {
	started, release := make(chan string, 4), make(chan struct{})
	testWork = func(ctx context.Context, dir job.Dir) (int, error) {
		started <- dir.Path()
		select {
		case <-release:
		case <-ctx.Done():
		}
		return 0, nil
	}
	t.Cleanup(func() { testWork = nil })
	site := open(t)
	// Registered after the site, so it runs before the queue is stopped, which waits for a turn that is still answering.
	t.Cleanup(func() { close(release) })
	site.signIn(t)
	id, _ := aRoutine(t, site, url.Values{"title": {"Hourly"}, "prompt": {"go"}, "every": {"1"}})
	routine := site.server.instances.Routines(id)[0]
	site.server.StartDueRoutines(time.Now())
	thread, _ := site.server.instances.Thread(id, routine.Thread)
	if len(thread.Turns) != 1 {
		t.Fatalf("turns %d", len(thread.Turns))
	}
	<-started
	site.server.StartDueRoutines(time.Now().Add(2 * time.Hour))
	if again, _ := site.server.instances.Thread(id, routine.Thread); len(again.Turns) != 1 {
		t.Errorf("a second turn started beside one that was still answering: %d", len(again.Turns))
	}
	if last := site.server.instances.Routines(id)[0].Last; last == nil || !strings.Contains(last.Skipped, "still answering") {
		t.Errorf("the skip is not kept: %+v", last)
	}
}

func TestAPausedRoutineStartsNothingUntilItIsResumed(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, where := aRoutine(t, site, url.Values{"title": {"Paused"}, "prompt": {"go"}, "every": {"1"}})
	routine := site.server.instances.Routines(id)[0]
	site.ask(t, "POST", where+"/routines/"+routine.ID+"/pause", nil)
	site.server.StartDueRoutines(time.Now())
	if thread, _ := site.server.instances.Thread(id, routine.Thread); len(thread.Turns) != 0 {
		t.Fatalf("a paused routine started %d turns", len(thread.Turns))
	}
	site.ask(t, "POST", where+"/routines/"+routine.ID+"/resume", nil)
	site.server.StartDueRoutines(time.Now())
	if thread, _ := site.server.instances.Thread(id, routine.Thread); len(thread.Turns) != 1 {
		t.Fatalf("a resumed routine started %d turns", len(thread.Turns))
	}
	site.ask(t, "POST", where+"/routines/"+routine.ID+"/remove", nil)
	if len(site.server.instances.Routines(id)) != 0 {
		t.Error("the routine was not removed")
	}
}

func TestARoutineWithNoScheduleOrNoQuestionIsRefusedOnThePage(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedInstance(t, site)
	for name, form := range map[string]url.Values{
		"no schedule": {"title": {"x"}, "prompt": {"y"}},
		"no prompt":   {"title": {"x"}, "daily": {"08:00"}},
		"both":        {"title": {"x"}, "prompt": {"y"}, "daily": {"08:00"}, "every": {"2"}},
		"bad time":    {"title": {"x"}, "prompt": {"y"}, "daily": {"8am"}},
	} {
		if location := site.ask(t, "POST", where+"/routines", form).Header().Get("Location"); !strings.Contains(location, "refused=") {
			t.Errorf("%s was taken: %q", name, location)
		}
	}
}

func TestSomebodyElsesAgentTakesNoRoutineAndAnswersAsAMissingOneDoes(t *testing.T) {
	site := open(t)
	site.signIn(t)
	if got := site.ask(t, "POST", "/instances/nobody-here/routines", url.Values{"title": {"x"}, "prompt": {"y"}, "every": {"1"}}).Code; got != http.StatusNotFound {
		t.Errorf("answered %d", got)
	}
}
