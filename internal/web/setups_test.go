package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/changelog"
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
	if page := site.ask(t, "GET", "/setups/reviewer", nil); page.Code != http.StatusNotFound {
		t.Fatalf("a skill set that is off is shown to people: %d", page.Code)
	}
	if started := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "setup": {"reviewer"}}); !strings.Contains(started.Header().Get("Location"), "refused=") {
		t.Fatalf("an agent was started with a skill set that is off: %q", started.Header().Get("Location"))
	}
	if list := site.ask(t, "GET", "/new/blank", nil).Body.String(); strings.Contains(list, "reviewer") {
		t.Error("a skill set that is off is offered to people")
	}
	if console := site.ask(t, "GET", "/admin/skill-sets", nil).Body.String(); !strings.Contains(console, "reviewer") || !strings.Contains(console, "off") {
		t.Error("the console does not show the skill set that is off")
	}
	site.ask(t, "POST", "/admin/skill-sets/reviewer/enable", nil)
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
	site.ask(t, "POST", "/admin/skill-sets/nothing-fits/enable", nil)
	answer := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "setup": {"no-such-setup"}})
	if !strings.Contains(answer.Header().Get("Location"), "refused=") {
		t.Errorf("a setup that does not exist was accepted: %q", answer.Header().Get("Location"))
	}
	if made := site.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"x"}, "server": {"not-offered"}}); !strings.Contains(made.Header().Get("Location"), "refused=") {
		t.Error("a setup named a server the deployment does not offer")
	}
}

func TestASkillIsPutBackThroughThePageAndTheRecordSaysWhatItWasAndBecame(t *testing.T) {
	admin, log := recording(t)
	admin.signIn(t)
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}})
	admin.ask(t, "POST", "/admin/skill-sets/reviewer/enable", nil)
	write := func(body string) {
		admin.ask(t, "POST", "/admin/skill-sets/reviewer/skills", url.Values{"skill": {"review"}, "description": {"Use to review."}, "body": {body}})
	}
	write("first words")
	write("second words")
	page := admin.ask(t, "GET", "/admin/skill-sets/reviewer", nil).Body.String()
	if !strings.Contains(page, "Earlier versions") || !strings.Contains(page, "Put it back") {
		t.Fatalf("the page does not offer the earlier version:\n%s", page)
	}
	at := regexp.MustCompile(`name="at" value="(\d+)"`).FindStringSubmatch(page)
	if at == nil {
		t.Fatal("no version to put back")
	}
	admin.ask(t, "POST", "/admin/skill-sets/reviewer/skills/review/restore", url.Values{"at": {at[1]}})
	held, _ := admin.server.setups.Get("reviewer")
	if len(held.Skills) != 1 || held.Skills[0].Body != "first words" {
		t.Fatalf("after putting it back: %+v", held.Skills)
	}
	events := written(t, log)
	var kinds []string
	for _, event := range events {
		kinds = append(kinds, event.Kind)
		if event.Kind == changelog.SkillWritten || event.Kind == changelog.SkillRestored {
			if len(event.What["was"]) == 0 || len(event.What["now"]) == 0 || event.What["was"] == event.What["now"] {
				t.Errorf("%s does not say what it was and became: %+v", event.Kind, event.What)
			}
			if strings.Contains(strings.Join(mapValues(event.What), " "), "words") {
				t.Errorf("the record holds the skill's text: %+v", event.What)
			}
		}
	}
	for _, want := range []string{changelog.SetupEnabled, changelog.SkillRestored} {
		if !slices.Contains(kinds, want) {
			t.Errorf("the record lacks %s: %v", want, kinds)
		}
	}
	if refused := admin.ask(t, "POST", "/admin/skill-sets/reviewer/skills/review/restore", url.Values{"at": {"1"}}); !strings.Contains(refused.Header().Get("Location"), "refused=") {
		t.Error("a version that was never kept was put back")
	}
	admin.ask(t, "POST", "/admin/skill-sets/reviewer/disable", nil)
	if held, _ := admin.server.setups.Get("reviewer"); !held.Off {
		t.Error("turning it off did not keep")
	}
}

func mapValues(m map[string]string) []string {
	var out []string
	for _, value := range m {
		out = append(out, value)
	}
	return out
}
