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

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/jobs"
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
	t.Setenv("GUSTO_HARNESSES", adapters)
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

// The fastest and the cheapest are chosen among the runs that passed, and only
// when they stand clear of the others.
func TestOnlyAPassingRunIsBestAtAnything(t *testing.T) {
	sides := []side{
		{Passed: false, Record: jobs.Record{Started: 1, Ended: 2}, Metered: true},
		{Passed: true, Record: jobs.Record{Started: 1, Ended: 30}, Metered: true},
		{Passed: true, Record: jobs.Record{Started: 1, Ended: 10}, Metered: true},
	}
	sides[1].Spent.USD, sides[2].Spent.USD = 0.001, 0.002
	weigh(sides)
	if sides[0].Fastest || sides[0].Cheapest || !sides[2].Fastest || !sides[1].Cheapest {
		t.Errorf("best = %+v", sides)
	}
}

func TestRunsThatAreLevelHaveNoBest(t *testing.T) {
	sides := []side{
		{Passed: true, Record: jobs.Record{Started: 0 + 1, Ended: 101}, Metered: true},
		{Passed: true, Record: jobs.Record{Started: 1, Ended: 106}, Metered: true},
	}
	sides[0].Spent.USD, sides[1].Spent.USD = 1.00, 1.05
	reading := weigh(sides)
	if sides[0].Fastest || sides[1].Fastest || sides[0].Cheapest || sides[1].Cheapest {
		t.Errorf("a best was named between level runs: %+v", sides)
	}
	if !strings.Contains(reading.Headline, "Nothing separates them") {
		t.Errorf("headline %q", reading.Headline)
	}
}

// A finished comparison says which run did better, at what cost and in how
// long, and a comparison with a run still working says none of it yet.
func TestTheComparisonPageSaysWhichDidBetter(t *testing.T) {
	site := twoEngines(t)
	board := site.server.lineups
	lineup := compare.Lineup{ID: "two-runs", Brief: "fix it", Model: "m"}
	for i, each := range []struct {
		harness string
		passed  bool
		took    int64
		lines   []string
	}{
		{"first", true, 40, []string{priced, priced, priced}},
		{"second", false, 90, []string{priced}},
	} {
		id := fmt.Sprintf("run-%d", i)
		spentJob(t, site.index.Root(), id, "", each.lines...)
		dir := job.Dir(filepath.Join(site.index.Root(), id))
		if err := evidence.Write(dir, evidence.Manifest{Gate: &evidence.Verdict{Passed: each.passed}}); err != nil {
			t.Fatal(err)
		}
		if err := site.index.Add(jobs.Record{ID: id, State: jobs.Passed, Brief: "fix it", Harness: each.harness, Started: 100, Ended: 100 + each.took}); err != nil {
			t.Fatal(err)
		}
		lineup.Runs = append(lineup.Runs, compare.Entry{Harness: each.harness, Job: id})
	}
	if err := board.Save(lineup); err != nil {
		t.Fatal(err)
	}
	site.signIn(t)
	page := site.ask(t, "GET", "/compare/two-runs", nil).Body.String()
	for _, want := range []string{"First was the only one to pass the check.", ">Model calls</th>", ">3</td>", "$0.7500", "40 s"} {
		if !strings.Contains(page, want) {
			t.Errorf("want %q on the page:\n%s", want, page)
		}
	}

	if err := job.Dir(filepath.Join(site.index.Root(), "run-2")).Make(); err != nil {
		t.Fatal(err)
	}
	if err := site.index.Add(jobs.Record{ID: "run-2", State: jobs.Running, Brief: "fix it", Harness: "second", Started: 100}); err != nil {
		t.Fatal(err)
	}
	unfinished := compare.Lineup{ID: "still-going", Brief: "fix it", Model: "m",
		Runs: []compare.Entry{lineup.Runs[0], {Harness: "second", Job: "run-2"}}}
	if err := board.Save(unfinished); err != nil {
		t.Fatal(err)
	}
	if page := site.ask(t, "GET", "/compare/still-going", nil).Body.String(); strings.Contains(page, "only one to pass") {
		t.Errorf("a verdict was given while a run was still working:\n%s", page)
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

// The engine choices are their own control. Under the catalog's card class
// they drew as tall pills of every width, with the checkbox above the name.
func TestEngineChoicesDoNotWearTheCatalogCardClass(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	page := site.ask(t, "GET", "/compare", nil).Body.String()
	if strings.Count(page, ` data-engine>`) != 2 || strings.Contains(page, `<label class="pick"`) {
		t.Fatalf("the engine choices are not their own control:\n%s", page)
	}
}
