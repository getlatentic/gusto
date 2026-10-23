package web

import (
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

// aTask runs a task on a freshly started instance and says which job it made.
func aTask(t *testing.T, site *site, form url.Values) string {
	t.Helper()
	where := startedInstance(t, site)
	answer := site.ask(t, "POST", where+"/tasks", form)
	id, found := strings.CutPrefix(answer.Header().Get("Location"), "/jobs/")
	if answer.Code != http.StatusSeeOther || !found {
		t.Fatalf("the task was not taken: %d to %q", answer.Code, answer.Header().Get("Location"))
	}
	return id
}

// taskSpec is the job.json a task left behind.
func taskSpec(t *testing.T, site *site, id string) job.Spec {
	t.Helper()
	record, found := site.index.Get(id)
	if !found {
		t.Fatalf("no job called %q", id)
	}
	spec, err := job.Read(record.Dir(site.index.Root()))
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

// The task form is the one way a page runs a gated task, so it carries
// everything the form it replaced did: the spend is counted against the team
// and stopped at the cap, and the rules read the sensitivity.
func TestATaskCarriesItsSensitivityItsTeamAndItsCap(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{
		"brief": {"do it"}, "gate": {"true"},
		"sensitivity": {"restricted"}, "team": {" platform "}, "usd": {"$2.50"},
	})
	spec := taskSpec(t, site, id)
	if spec.Sensitivity != job.Restricted {
		t.Errorf("sensitivity %q", spec.Sensitivity)
	}
	if spec.Team != "platform" || spec.Budget.USD == nil || *spec.Budget.USD != 2.5 {
		t.Errorf("team %q, cap %v", spec.Team, spec.Budget.USD)
	}
}

// Nothing picked and nothing typed is what a job naming none is read as: an
// internal job with no cap, which is not a cap of nothing.
func TestATaskThatSaysNothingIsInternalAndUncapped(t *testing.T) {
	site := open(t)
	site.signIn(t)
	spec := taskSpec(t, site, aTask(t, site, url.Values{"brief": {"do it"}, "gate": {"true"}}))
	if spec.Sensitivity != job.Internal || spec.Budget.USD != nil || spec.Team != "" {
		t.Errorf("sensitivity %q, team %q, cap %v", spec.Sensitivity, spec.Team, spec.Budget.USD)
	}
}

// A quoted gate is one argument, the same as the command line reads it.
func TestAQuotedGateSurvivesTheTaskForm(t *testing.T) {
	site := open(t)
	site.signIn(t)
	spec := taskSpec(t, site, aTask(t, site, url.Values{"brief": {"do it"}, "gate": {`sh -c "test 2 = 2"`}}))
	if len(spec.Gate) != 3 || spec.Gate[2] != "test 2 = 2" {
		t.Fatalf("a quoted gate must survive the form: %#v", spec.Gate)
	}
}

// A cap nothing could spend under, or a label the rules do not know, is
// refused rather than written into a job that would then fail to run. An
// unknown label read as "not restricted" would switch the one rule off.
func TestATaskTheRunnerWouldRefuseIsRefusedHere(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedInstance(t, site)
	for _, wrong := range []url.Values{
		{"usd": {"five"}}, {"usd": {"-1"}}, {"usd": {"NaN"}},
		{"sensitivity": {"secret"}}, {"sensitivity": {"Restricted"}},
	} {
		wrong.Set("brief", "do it")
		wrong.Set("gate", "true")
		answer := site.ask(t, "POST", where+"/tasks", wrong)
		if !strings.Contains(answer.Header().Get("Location"), "refused") {
			t.Errorf("%v was sent to %q", wrong, answer.Header().Get("Location"))
		}
	}
	if len(site.index.List()) != 0 {
		t.Fatal("a refused task left a directory behind")
	}
}

// A one-off against a repository is an instance started with that repository,
// and whoever asked for one from the record lands on the task form rather than
// in a conversation.
func TestAOneOffAgainstARepositoryIsAnInstanceStartedForATask(t *testing.T) {
	site := open(t)
	site.signIn(t)
	setup := site.ask(t, "GET", "/instances/new?harness=fixture&then=task", nil).Body.String()
	if !strings.Contains(setup, `name="then" value="task"`) {
		t.Fatal("the set-up page forgets that a task was asked for")
	}
	repo := aRepository(t)
	site.server.UseLocalRepos([]string{filepath.Dir(repo)})
	started := site.ask(t, "POST", "/instances", url.Values{
		"harness": {"fixture"}, "repo": {repo}, "then": {"task"},
	})
	where, found := strings.CutSuffix(started.Header().Get("Location"), "#task")
	if started.Code != http.StatusSeeOther || !found {
		t.Fatalf("starting for a task sent to %d %q", started.Code, started.Header().Get("Location"))
	}
	body := site.ask(t, "GET", where, nil).Body.String()
	for _, want := range []string{repo, `id="task"`, `name="sensitivity"`, `name="team"`, `name="usd"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the instance page does not carry %s", want)
		}
	}
	answer := site.ask(t, "POST", where+"/tasks", url.Values{"brief": {"fix it"}, "gate": {"true"}})
	id := strings.TrimPrefix(answer.Header().Get("Location"), "/jobs/")
	if record, _ := site.index.Get(id); record.Repo != repo {
		t.Errorf("the task does not name the repository it ran against: %+v", record)
	}
}

// aRepository is a repository on disk with one commit, which is all a clone
// needs.
func aRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "."},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--quiet", "--allow-empty", "-m", "first"},
	} {
		if output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", args[0], err, output)
		}
	}
	return dir
}
