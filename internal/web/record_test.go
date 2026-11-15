package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The record starts nothing. A run happens on an instance, and the record's page says where to start one when it has nothing
// to list (web/src/pages/work/activity-page.test.tsx).
func TestTheRecordStartsNothingAndSaysWhereToStart(t *testing.T) {
	site := open(t)
	site.signIn(t)
	isTheApp(t, site, "/jobs")
	if board := site.activity(t, ""); len(board.runs()) != 0 || board.State != "" || board.Harness != "" {
		t.Errorf("an empty record lists %+v", board)
	}
	if posted := site.ask(t, "POST", "/jobs", url.Values{"brief": {"x"}, "gate": {"true"}}); posted.Code != http.StatusMethodNotAllowed {
		t.Errorf("posting to the record gave %d", posted.Code)
	}
	if len(site.index.List()) != 0 {
		t.Fatal("posting to the record made a job")
	}
}

// A task names the instance it ran on, in the record and on its own page, and both link to it.
func TestARunLinksToTheInstanceItRanOn(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{"brief": {"count the files"}, "gate": {"true"}})
	record, _ := site.index.Get(id)
	if on := site.activity(t, "").run(t, id).On; on.Link != "/instances/"+record.Instance {
		t.Errorf("Activity does not link %s to the instance %s: %+v", id, record.Instance, on)
	}
	if page := site.task(t, id); page.Agent == nil || page.Agent.ID != record.Instance {
		t.Errorf("the task's page does not link to the instance %s: %+v", record.Instance, page.Agent)
	}
}

// A task sent from somewhere else ran on no instance, and the record says how it arrived rather than drawing an empty link.
func TestATaskSentOverHTTPSaysSoInsteadOfAnInstance(t *testing.T) {
	site := onlyBuiltIn(t)
	site.signIn(t)
	code, answer := site.send(t, password, Submission{Spec: aSpec()})
	if code != http.StatusCreated {
		t.Fatalf("code %d: %s", code, answer.Refused)
	}
	listed := site.activity(t, "").run(t, answer.ID).On
	page := site.task(t, answer.ID)
	for where, on := range map[string]origin{"Activity": listed, "the task's page": page.On} {
		if on.Named != "API" || !strings.Contains(on.Why, "sent to POST /api/jobs") || on.Link != "" {
			t.Errorf("%s does not say the job was sent over HTTP: %+v", where, on)
		}
	}
	if page.Agent != nil {
		t.Errorf("the task's page names an instance the job never ran on: %+v", page.Agent)
	}
}

// Nothing that ran before runs recorded their instance becomes unreadable. The directory below is what the old task form
// left: a state with no instance, a spec, and a manifest carrying a cost.
func TestATaskFromBeforeInstancesStillListsAndOpens(t *testing.T) {
	root := t.TempDir()
	const id = "fix-the-adder-20260901-1200"
	leaveOld(t, filepath.Join(root, id), map[string]string{
		"state.json": `{"id":"` + id + `","state":"passed","brief":"fix the adder","repo":"/repo",` +
			`"harness":"fixture","asked":"someone@example.com","exit":0,"queuedAt":1788264000,` +
			`"startedAt":1788264001,"endedAt":1788264060}`,
		"job.json": `{"version":1,"brief":"fix the adder","repo":"/repo","ref":"HEAD","gate":["true"],` +
			`"harness":"fixture","sensitivity":"internal","budget":{"turns":40,"seconds":900}}`,
		"out/manifest.json": `{"version":1,"job":"","files":[],"gate":{"command":["true"],"code":0,"passed":true},` +
			`"harness":"fixture","harnessExit":0,"stoppedAtBudget":false,"modelEndpoint":"local",` +
			`"tokens":null,"costUsd":0.5,"signer":"","atUnix":1788264060}`,
	})
	site := openAt(t, root)
	site.signIn(t)

	listed := site.activity(t, "").run(t, id)
	if listed.Brief != "fix the adder" || listed.Cost == nil || listed.Cost.Text != "$0.5000" ||
		listed.On.Why != "none: made before runs belonged to an instance" {
		t.Errorf("the record does not show the old task as it was: %+v", listed)
	}
	if opened := site.ask(t, "GET", "/jobs/"+id, nil); opened.Code != http.StatusOK {
		t.Fatalf("the old job does not open: %d", opened.Code)
	}
	if on := site.task(t, id).On; on.Why != "none: made before runs belonged to an instance" {
		t.Errorf("the old job's page does not say it ran on no instance: %+v", on)
	}
}

// A run whose instance this server no longer holds names it and links nowhere.
func TestARunOnAnInstanceNoLongerHeldNamesItWithoutALink(t *testing.T) {
	root := t.TempDir()
	const id = "gone-20260901-1200"
	leaveOld(t, filepath.Join(root, id), map[string]string{
		"state.json": `{"id":"` + id + `","state":"failed","brief":"gone","repo":"","harness":"fixture",` +
			`"instance":"somewhere-else","asked":"someone","exit":1,"queuedAt":1788264000}`,
	})
	site := openAt(t, root)
	site.signIn(t)
	page := site.task(t, id)
	if page.On.Why != "somewhere-else, which this server no longer holds" || page.On.Named != "removed" {
		t.Errorf("the page does not name the instance it no longer holds: %+v", page.On)
	}
	if page.On.Link != "" || page.Agent != nil {
		t.Error("the page links to an instance that is not there")
	}
}

// leaveOld writes files into a task directory exactly as given, so the test
// reads bytes an older server wrote rather than what this one would write.
func leaveOld(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
