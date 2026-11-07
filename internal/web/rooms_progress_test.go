package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/tasks"
)

// aStep is a line of a turn's commands, as an agent's harness writes them.
func aStep(t *testing.T, site *site, id string, steps ...string) {
	t.Helper()
	record, found := site.index.Get(id)
	if !found {
		t.Fatalf("no turn %s", id)
	}
	if err := os.MkdirAll(record.Dir(site.index.Root()).Out(), 0o755); err != nil {
		t.Fatal(err)
	}
	var lines strings.Builder
	for _, path := range steps {
		lines.WriteString(`{"tool":"read","input":{"path":"` + path + `"}}` + "\n")
	}
	if err := os.WriteFile(record.Dir(site.index.Root()).OutFile("commands.jsonl"), []byte(lines.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runningIn(site *site) []tasks.Record {
	var running []tasks.Record
	for _, record := range site.index.List() {
		if record.State == tasks.Running {
			running = append(running, record)
		}
	}
	return running
}

func TestEachAgentWorkingAtOnceHasItsOwnProgress(t *testing.T) {
	work := newGatedWork()
	withWorkers(t, 2, work)
	site, _, thread := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	mine := site.as(t, alice)
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout and @Draft, go"}})
	work.waitEntered(t, 2)
	running := runningIn(site)
	if len(running) != 2 {
		t.Fatalf("%d turns are running, not two", len(running))
	}
	aStep(t, site, running[0].ID, "brief.md")
	aStep(t, site, running[1].ID, "brief.md", "notes.md", "policy.md")

	asked := mine.ask(t, "GET", thread+"/progress", nil)
	if asked.Code != http.StatusOK || asked.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("progress gave %d with cache %q", asked.Code, asked.Header().Get("Cache-Control"))
	}
	var doing map[string]turnProgress
	if err := json.Unmarshal(asked.Body.Bytes(), &doing); err != nil {
		t.Fatal(err)
	}
	if len(doing) != 2 {
		t.Fatalf("progress for %d turns, not both: %v", len(doing), doing)
	}
	for _, record := range running {
		if doing[record.ID].What != "Read" {
			t.Errorf("turn %s is doing %+v", record.ID, doing[record.ID])
		}
	}
	steps := []int{doing[running[0].ID].Steps, doing[running[1].ID].Steps}
	if steps[0] != 1 || steps[1] != 3 || doing[running[1].ID].Detail != "policy.md" {
		t.Errorf("each turn is told apart: %v and %+v", steps, doing[running[1].ID])
	}
	work.release()
	waitIdle(t, site)
	if after := mine.ask(t, "GET", thread+"/progress", nil).Body.String(); strings.TrimSpace(after) != "{}" {
		t.Errorf("turns that ended still have progress: %s", after)
	}
}

func TestATurnThatHasNotStartedHasNoProgressAndAPersonOutsideTheRoomIsRefused(t *testing.T) {
	work := newGatedWork()
	withWorkers(t, 1, work)
	site, _, thread := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	mine := site.as(t, alice)
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout and @Draft, go"}})
	work.waitEntered(t, 1)

	var doing map[string]turnProgress
	if err := json.Unmarshal(mine.ask(t, "GET", thread+"/progress", nil).Body.Bytes(), &doing); err != nil {
		t.Fatal(err)
	}
	if len(doing) != 1 {
		t.Errorf("one turn is running and one waits for a runner, and progress is for %d: %v", len(doing), doing)
	}
	if got := site.as(t, bob).ask(t, "GET", thread+"/progress", nil); got.Code != http.StatusNotFound {
		t.Errorf("a person outside the room was given its progress: %d", got.Code)
	}
	work.release()
	waitIdle(t, site)
}

func TestProgressIsReadOnceForEveryoneAskingWithinAMoment(t *testing.T) {
	var kept progressKept
	now := time.Now()
	kept.put("t1", turnProgress{What: "Read", Steps: 2}, now)
	if got, fresh := kept.get("t1", now.Add(progressLife/2)); !fresh || got.Steps != 2 {
		t.Errorf("progress read a moment ago was read again: %+v %v", got, fresh)
	}
	if _, fresh := kept.get("t1", now.Add(2*progressLife)); fresh {
		t.Error("progress older than its life was served")
	}
	for i := range mostProgress + 1 {
		kept.put(strings.Repeat("x", i%7)+string(rune('a'+i%26))+time.Duration(i).String(), turnProgress{}, now)
	}
	kept.guard.Lock()
	size := len(kept.held)
	kept.guard.Unlock()
	if size > mostProgress {
		t.Errorf("%d kept, past the bound of %d", size, mostProgress)
	}
}
