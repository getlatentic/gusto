package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// A setup is made once and started with again: every turn on an instance
// started with it carries its skills and its servers into the job, and
// job.json names them without the servers' definitions.
func TestEveryTurnOnAnInstanceCarriesItsSetup(t *testing.T) {
	site := open(t)
	site.signIn(t)
	made := site.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}, "server": {"memory"}})
	if made.Code != http.StatusSeeOther || made.Header().Get("Location") != "/admin/skill-sets/reviewer" {
		t.Fatalf("making a setup answered %d to %q", made.Code, made.Header().Get("Location"))
	}
	site.ask(t, "POST", "/admin/skill-sets/reviewer/skills", url.Values{
		"skill": {"review"}, "description": {"Use when asked to review."}, "body": {"Read the diff first."},
	})
	if page := site.ask(t, "GET", "/setups/reviewer", nil).Body.String(); !strings.Contains(page, "Use when asked to review.") {
		t.Fatal("the setup's page does not show its skill")
	}
	started := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "setup": {"reviewer"}})
	where := started.Header().Get("Location")
	said := site.ask(t, "POST", where+"/say", url.Values{"said": {"review this"}})
	id := strings.TrimPrefix(said.Header().Get("Location"), where+"#")
	site.wait(t, id)

	dir := job.Dir(filepath.Join(site.index.Root(), id))
	spec, err := job.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Setup == nil || spec.Setup.Name != "reviewer" || spec.Setup.Skills[0] != "review" || spec.Setup.Servers[0] != "memory" {
		t.Fatalf("job.json names %+v", spec.Setup)
	}
	written, _ := os.ReadFile(dir.Spec())
	if strings.Contains(string(written), "s3cret") {
		t.Error("a server's definition reached job.json")
	}
	if _, err := os.Stat(filepath.Join(dir.SetupDir(), "skills", "review", "SKILL.md")); err != nil {
		t.Error("the skill did not travel with the turn")
	}
}

// A setup a harness cannot take is not offered for it, and starting it anyway
// is refused before anything runs.
func TestASetupAHarnessCannotTakeIsRefused(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"nothing-fits"}, "server": {"memory"}})
	answer := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "setup": {"no-such-setup"}})
	if !strings.Contains(answer.Header().Get("Location"), "refused=") {
		t.Errorf("a setup that does not exist was accepted: %q", answer.Header().Get("Location"))
	}
	if made := site.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"x"}, "server": {"not-offered"}}); !strings.Contains(made.Header().Get("Location"), "refused=") {
		t.Error("a setup named a server the deployment does not offer")
	}
}
