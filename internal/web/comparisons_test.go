package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
)

// twoEngines is a server offering two harnesses that need no account.
func twoEngines(t *testing.T) *site {
	t.Helper()
	adapters := t.TempDir()
	var entries []catalog.Entry
	for _, name := range []string{"first", "second"} {
		text := strings.Replace(fmt.Sprintf(fixtureJSON, "echo done"), `"fixture"`, `"`+name+`"`, 1)
		put(t, filepath.Join(adapters, name+".json"), text, 0o644)
		entries = append(entries, catalog.Entry{Name: name, Title: strings.ToUpper(name[:1]) + name[1:], Program: "sh",
			Needs: harness.NeedsNothing, Ready: true, Skills: true, MCP: true})
	}
	t.Setenv("HAAS_HARNESSES", adapters)
	site := openOffering(t, entries)
	board, err := compare.OpenBoard(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site.server.UseBoard(board)
	return site
}

// One task goes to each engine chosen: the same brief on the same commit of
// the sample, with the same model, skills and check, and the page shows each.
func TestAComparisonGivesEveryAgentTheSameTask(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	started := site.ask(t, "POST", "/compare", url.Values{
		"brief": {"Run the tests and fix what fails"}, "model": {modelOf("platform/test-model")},
		"harness": {"first", "second"},
	})
	where := started.Header().Get("Location")
	if started.Code != http.StatusSeeOther || !strings.HasPrefix(where, "/compare/") {
		t.Fatalf("starting a comparison answered %d to %q", started.Code, where)
	}
	lineup, found := site.server.lineups.Get(strings.TrimPrefix(where, "/compare/"))
	if !found || len(lineup.Runs) != 2 {
		t.Fatalf("lineup %+v", lineup)
	}
	var specs []job.Spec
	for _, run := range lineup.Runs {
		waitFor(t, site.index, run.Job)
		spec, err := job.Read(job.Dir(filepath.Join(site.index.Root(), run.Job)))
		if err != nil {
			t.Fatal(err)
		}
		specs = append(specs, spec)
	}
	first, second := specs[0], specs[1]
	if first.Harness == second.Harness || first.Ref != lineup.Commit || second.Ref != lineup.Commit ||
		first.Brief != second.Brief || first.Model != second.Model || strings.Join(first.Gate, " ") != "node --test" ||
		first.Setup == nil || first.Setup.Name != "engineering" {
		t.Fatalf("the runs were not given the same task:\n%+v\n%+v", first, second)
	}
	if _, err := os.Stat(filepath.Join(first.Repo, "invoice.test.js")); err != nil {
		t.Errorf("the sample does not hold the template's files: %v", err)
	}
	page := site.ask(t, "GET", where, nil).Body.String()
	for _, want := range []string{"First", "Second", "Only the agent differs"} {
		if !strings.Contains(page, want) {
			t.Errorf("want %q on the page:\n%s", want, page)
		}
	}
}

func TestAComparisonNeedsTwoAgents(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	refused := site.ask(t, "POST", "/compare", url.Values{"brief": {"x"}, "model": {modelOf("platform/test-model")}, "harness": {"first"}})
	if !strings.Contains(refused.Header().Get("Location"), "refused=") {
		t.Fatal("a comparison of one agent was started")
	}
}

// The fastest and the cheapest are chosen among the runs that passed.
func TestOnlyAPassingRunIsBestAtAnything(t *testing.T) {
	sides := []side{
		{Passed: false, Record: jobs.Record{Started: 1, Ended: 2}, Metered: true},
		{Passed: true, Record: jobs.Record{Started: 1, Ended: 30}, Metered: true},
		{Passed: true, Record: jobs.Record{Started: 1, Ended: 10}, Metered: true},
	}
	sides[1].Spent.USD, sides[2].Spent.USD = 0.001, 0.002
	markBest(sides)
	if sides[0].Fastest || sides[0].Cheapest || !sides[2].Fastest || !sides[1].Cheapest {
		t.Errorf("best = %+v", sides)
	}
}

func waitFor(t *testing.T, index *jobs.Index, id string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if record, found := index.Get(id); found && record.State.Done() {
			return
		}
	}
	t.Fatalf("%s never finished", id)
}
