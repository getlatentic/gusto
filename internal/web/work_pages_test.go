package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/harness"
)

// Each work page is the app's, and its data, at its own address; one whose address names nothing there answers 404 for both.
func TestEachWorkPageIsTheAppAndItsDataAt404WhenNothingIsThere(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	started := site.ask(t, "POST", "/instances", url.Values{"harness": {"first"}}).Header().Get("Location")
	where, _, _ := strings.Cut(started, "/threads/")
	site.leave(t, "one-task", leftBehind{state: tasks.Passed, out: map[string]string{}, check: passed})
	if err := site.server.lineups.Save(compare.Lineup{ID: "one-lineup", Brief: "fix the parser", Who: sharedSignIn}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/jobs", "/jobs/one-task", where, "/engines", "/compare", "/compare/one-lineup"} {
		isTheApp(t, site, path)
		if got := site.ask(t, "GET", "/app/api"+path, nil); got.Code != http.StatusOK || got.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("the data of %s answered %d", path, got.Code)
		}
	}
	for _, path := range []string{"/jobs/no-such-task", "/instances/no-such-agent", "/compare/no-such-comparison"} {
		for _, at := range []string{path, "/app/api" + path} {
			if got := site.ask(t, "GET", at, nil); got.Code != http.StatusNotFound {
				t.Errorf("%s answered %d", at, got.Code)
			}
		}
	}
}

// A server with nowhere to keep comparisons has none to show.
func TestWithoutABoardAComparisonIsNotFound(t *testing.T) {
	site := open(t)
	site.signIn(t)
	for _, at := range []string{"/compare/anything", "/app/api/compare/anything"} {
		if got := site.ask(t, "GET", at, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", at, got.Code)
		}
	}
	var page compareJSON
	site.data(t, "/compare", &page)
	if page.Lineups == nil || len(page.Lineups) != 0 {
		t.Errorf("a server with no board lists %+v", page.Lineups)
	}
}

// A refused comparison is said on the page it was started from.
func TestARefusedComparisonIsSaidOnTheComparePage(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	refused := site.ask(t, "POST", "/compare", url.Values{"brief": {""}, "harness": {"first", "second"}})
	landed := refused.Header().Get("Location")
	if refused.Code != http.StatusSeeOther || !strings.HasPrefix(landed, "/compare?") {
		t.Fatalf("a refused comparison answered %d to %q", refused.Code, landed)
	}
	var page compareJSON
	site.data(t, landed, &page)
	if page.Refused != "say what the agents should do" {
		t.Errorf("the compare page says %q", page.Refused)
	}
}

// How each agent type did is a grid of its runs on each model, each row with the agent type's name and mark.
func TestHowEachAgentDidIsAGridOfAgentTypesByModel(t *testing.T) {
	seconds, cost := 42.0, 0.0123
	site := openOffering(t, []catalog.Entry{
		{Name: "fixture", Title: "Fixture", Program: "sh", Needs: harness.NeedsNothing, Ready: true, Measured: []catalog.Measured{
			{Model: "platform/test-model", Finished: true, WallSeconds: &seconds, CostUSD: &cost},
			{Model: "platform/other-model", Finished: false, WallSeconds: &seconds},
		}},
		{Name: "other", Title: "Other", Program: "sh", Needs: harness.NeedsNothing, Ready: true, Measured: []catalog.Measured{
			{Model: "platform/other-model", Finished: true, WallSeconds: &seconds},
		}},
	})
	site.signIn(t)
	var page enginesJSON
	site.data(t, "/engines", &page)
	grid := page.Measured
	if len(grid.Models) != 2 || grid.Finished != 2 || grid.Tried != 3 || len(grid.Rows) != 2 {
		t.Fatalf("the grid is %+v", grid)
	}
	first := grid.Rows[0]
	if first.Name != "fixture" || first.Title != "Fixture" || first.Cells[0] != (measuredCell{Tried: true, Finished: true, Took: "42 s", Cost: "$0.0123"}) ||
		first.Cells[1] != (measuredCell{Tried: true, Took: "42 s"}) {
		t.Errorf("the first row is %+v", first)
	}
	if other := grid.Rows[1]; other.Cells[0].Tried || !other.Cells[1].Finished {
		t.Errorf("a pair no matrix ran is not said to be untried: %+v", other)
	}
}

// Activity says how each run ended in the words of a log, and what it cost: the proxy's count when there is one.
func TestActivitySaysHowEachRunEndedAndWhatItCost(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "checked", leftBehind{state: tasks.Passed, out: map[string]string{}, check: passed})
	site.leave(t, "broken", leftBehind{state: tasks.Failed, out: map[string]string{}, check: passed})
	board := site.activity(t, "")
	if run := board.run(t, "checked"); run.Outcome != "check passed" || run.State != "passed" || run.Hour == "" || run.When == "" {
		t.Errorf("a passed task reads %+v", run)
	}
	if run := board.run(t, "broken"); run.Outcome != "check failed" || run.Cost != nil {
		t.Errorf("a failed task reads %+v", run)
	}
	if len(board.Days) != 1 || board.Days[0].Named != "Today" {
		t.Errorf("the runs are not under the day they were asked: %+v", board.Days)
	}
	for _, record := range []tasks.Record{
		{State: tasks.Queued}, {State: tasks.Running}, {State: tasks.Interrupted}, {State: tasks.Passed, Thread: "1"}, {State: tasks.Failed, Thread: "1"},
	} {
		want := map[tasks.State]string{tasks.Queued: "waiting", tasks.Running: "working", tasks.Interrupted: "interrupted", tasks.Passed: "replied", tasks.Failed: "failed"}[record.State]
		if got := outcomeOf(record); got != want {
			t.Errorf("a %s run reads %q, want %q", record.State, got, want)
		}
	}
}
