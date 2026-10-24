package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The record starts nothing. A run happens on an instance, and the record says
// where to start one.
func TestTheRecordStartsNothingAndSaysWhereToStart(t *testing.T) {
	site := open(t)
	site.signIn(t)
	body := site.ask(t, "GET", "/jobs", nil).Body.String()
	for _, gone := range []string{`name="brief"`, `name="gate"`, `method="post" action="/jobs"`} {
		if strings.Contains(body, gone) {
			t.Errorf("the record still carries %s", gone)
		}
	}
	if !strings.Contains(body, `href="/presets"`) {
		t.Error("the record does not say where a harness is started")
	}
	if posted := site.ask(t, "POST", "/jobs", url.Values{"brief": {"x"}, "gate": {"true"}}); posted.Code != http.StatusMethodNotAllowed {
		t.Errorf("posting to the record gave %d", posted.Code)
	}
	if len(site.index.List()) != 0 {
		t.Fatal("posting to the record made a job")
	}
}

// A task names the instance it ran on, in the record and on its own page, and
// both link to it.
func TestARunLinksToTheInstanceItRanOn(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{"brief": {"count the files"}, "gate": {"true"}})
	record, _ := site.index.Get(id)
	link := `href="/instances/` + record.Instance + `"`
	for _, page := range []string{"/jobs", "/jobs/" + id} {
		if body := site.ask(t, "GET", page, nil).Body.String(); !strings.Contains(body, link) {
			t.Errorf("%s does not link to the instance %s", page, record.Instance)
		}
	}
}

// A job sent from somewhere else ran on no instance, and the record says how
// it arrived rather than drawing an empty link.
func TestAJobSentOverHTTPSaysSoInsteadOfAnInstance(t *testing.T) {
	site := onlyBuiltIn(t)
	site.signIn(t)
	code, answer := site.send(t, password, Submission{Spec: aJob()})
	if code != http.StatusCreated {
		t.Fatalf("code %d: %s", code, answer.Refused)
	}
	for _, page := range []string{"/jobs", "/jobs/" + answer.ID} {
		body := site.ask(t, "GET", page, nil).Body.String()
		if !strings.Contains(body, "sent to POST /api/jobs") {
			t.Errorf("%s does not say the job was sent over HTTP", page)
		}
		// The only instance links the record may carry are the ones to start one.
		if strings.Count(body, `href="/instances/`) != strings.Count(body, `href="/instances/new?`) || strings.Contains(body, `href=""`) {
			t.Errorf("%s links to an instance the job never ran on", page)
		}
	}
}

// Nothing that ran before runs recorded their instance becomes unreadable. The
// directory below is what the old job form left: a state with no instance, a
// spec, and a manifest carrying a cost.
func TestAJobFromBeforeInstancesStillListsAndOpens(t *testing.T) {
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

	listed := site.ask(t, "GET", "/jobs", nil).Body.String()
	for _, want := range []string{"fix the adder", "/jobs/" + id, "$0.5000", "none: made before runs belonged to an instance"} {
		if !strings.Contains(listed, want) {
			t.Errorf("the record does not show %q", want)
		}
	}
	opened := site.ask(t, "GET", "/jobs/"+id, nil)
	if opened.Code != http.StatusOK {
		t.Fatalf("the old job does not open: %d", opened.Code)
	}
	if body := opened.Body.String(); !strings.Contains(body, "none: made before runs belonged to an instance") {
		t.Error("the old job's page does not say it ran on no instance")
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
	body := site.ask(t, "GET", "/jobs/"+id, nil).Body.String()
	if !strings.Contains(body, "somewhere-else, which this server no longer holds") {
		t.Error("the page does not name the instance it no longer holds")
	}
	if strings.Contains(body, `href="/instances/somewhere-else"`) {
		t.Error("the page links to an instance that is not there")
	}
}

// leaveOld writes files into a job directory exactly as given, so the test
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
