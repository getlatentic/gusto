package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// A template starts a harness set up for its field: its skills as a setup of
// its name, its sample files already in the folder, and its starters in the
// first thread.
func TestATemplateStartsAHarnessReadyForItsWork(t *testing.T) {
	site := open(t)
	site.signIn(t)
	if page := site.ask(t, "GET", "/new", nil).Body.String(); !strings.Contains(page, `href="/templates/health"`) {
		t.Fatal("the catalog does not offer the health template")
	}
	started := site.ask(t, "POST", "/templates/health", url.Values{"harness": {"fixture"}})
	where := started.Header().Get("Location")
	if started.Code != http.StatusSeeOther || !strings.Contains(where, "/threads/") {
		t.Fatalf("starting the template answered %d to %q", started.Code, where)
	}
	page := site.ask(t, "GET", where, nil).Body.String()
	if !strings.Contains(page, "Draft a SOAP note from visit-notes.txt") {
		t.Error("the first thread does not offer the template's starters")
	}
	if setup := site.ask(t, "GET", "/setups/health", nil).Body.String(); !strings.Contains(setup, "phi-guard") {
		t.Error("the template's skills are not a setup")
	}
	said := site.ask(t, "POST", where+"/say", url.Values{"said": {"Draft a SOAP note from visit-notes.txt"}})
	id := strings.TrimPrefix(said.Header().Get("Location"), where+"#")
	site.wait(t, id)
	spec, err := job.Read(job.Dir(filepath.Join(site.index.Root(), id)))
	if err != nil || spec.Setup == nil || spec.Setup.Name != "health" {
		t.Fatalf("the turn was given %+v, %v", spec.Setup, err)
	}
}

// A restricted template's turns are restricted, whichever thread they are
// said in, so the rule that keeps its work in the building applies to each.
func TestARestrictedTemplatesTurnsAreRestricted(t *testing.T) {
	site := open(t)
	site.signIn(t)
	refused := site.ask(t, "POST", "/templates/defence", url.Values{"harness": {"fixture"}})
	if !strings.Contains(refused.Header().Get("Location"), "refused=") {
		t.Fatal("restricted work started where no model is in the building")
	}
	site.server.UseInBuilding(true)
	started := site.ask(t, "POST", "/templates/defence", url.Values{"harness": {"fixture"}})
	where := started.Header().Get("Location")
	said := site.ask(t, "POST", where+"/say", url.Values{"said": {"List every requirement without a passing test"}})
	id := strings.TrimPrefix(said.Header().Get("Location"), where+"#")
	site.wait(t, id)
	spec, err := job.Read(job.Dir(filepath.Join(site.index.Root(), id)))
	if err != nil || spec.Sensitivity != job.Restricted {
		t.Fatalf("the turn's sensitivity is %q, %v", spec.Sensitivity, err)
	}
}

// A file in a harness's folder can be read from the page, and nothing outside
// the folder can: not a path that climbs out, not a link that points out, and
// not the folder's own git history.
func TestTheFolderViewerShowsOnlyTheFolder(t *testing.T) {
	site := open(t)
	site.signIn(t)
	started := site.ask(t, "POST", "/templates/health", url.Values{"harness": {"fixture"}})
	where := started.Header().Get("Location")
	instance := where[:strings.Index(where, "/threads/")]
	if page := site.ask(t, "GET", instance+"/file?path=visit-notes.txt", nil); page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "community-acquired pneumonia") {
		t.Fatalf("the sample file did not open: %d", page.Code)
	}
	for _, path := range []string{"../../../etc/passwd", "/etc/passwd", ".git/config", "missing.txt"} {
		if page := site.ask(t, "GET", instance+"/file?path="+url.QueryEscape(path), nil); page.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", path, page.Code)
		}
	}
}

// The new-agent page starts a role without asking which engine: the first one that can take it is used.
func TestARoleStartsOnTheFirstEngineThatCanTakeIt(t *testing.T) {
	site := open(t)
	site.signIn(t)
	started := site.ask(t, "POST", "/templates/health", url.Values{})
	if where := started.Header().Get("Location"); started.Code != http.StatusSeeOther || !strings.Contains(where, "/threads/") {
		t.Fatalf("starting with no engine chosen answered %d to %q", started.Code, where)
	}
}

func TestThePresetsPageShowsEachBuiltInPresetWhatItDoes(t *testing.T) {
	site := open(t)
	site.signIn(t)
	page := site.ask(t, "GET", "/new", nil).Body.String()
	for _, want := range []string{`data-role="health"`, `data-for="health"`, "What it can do", "Try asking", "Use in a new chat", `id="role-search"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the presets page does not carry %q", want)
		}
	}
}

func TestARoleStartsOnTheEngineChosenInItsFooter(t *testing.T) {
	site := open(t)
	site.signIn(t)
	started := site.ask(t, "POST", "/templates/health", url.Values{"engine": {"fixture"}})
	if where := started.Header().Get("Location"); started.Code != http.StatusSeeOther || !strings.Contains(where, "/threads/") {
		t.Fatalf("starting on a chosen engine answered %d to %q", started.Code, where)
	}
}
