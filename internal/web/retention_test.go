package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/retention"
)

// keeping is a site that removes finished work: a workspace after 7 days and a
// record after 90, with the change log on.
func keeping(t *testing.T) (*site, *changelog.Log, string) {
	t.Helper()
	here, log := recording(t)
	dir := t.TempDir()
	here.server.UseRetention(dir, retention.Policy{
		WorkspaceDays: map[string]int{"passed": 7}, RecordDays: map[string]int{"passed": 90},
	})
	return here, log, dir
}

// oldJob leaves a passed job that ended days ago, with a workspace.
func oldJob(t *testing.T, site *site, id string, days int) {
	t.Helper()
	spentJob(t, site.index.Root(), id, "", priced)
	work := filepath.Join(site.index.Root(), id, "workspace", "code.go")
	if err := os.MkdirAll(filepath.Dir(work), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(work, []byte(strings.Repeat("x", 2048)), 0o644); err != nil {
		t.Fatal(err)
	}
	ended := time.Now().AddDate(0, 0, -days).Unix()
	if err := site.index.Add(jobs.Record{ID: id, State: jobs.Passed, Brief: "brief " + id, Harness: "fixture",
		Asked: "amy@acme.test", Queued: ended, Started: ended, Ended: ended}); err != nil {
		t.Fatal(err)
	}
}

func TestTheRetentionPageIsAnAdminsAndOnlyWhereWorkIsRemoved(t *testing.T) {
	here, _, _ := keeping(t)
	oldJob(t, here, "a-week-old", 10)
	if got := here.as(t, "amy@acme.test").ask(t, "GET", "/admin/retention", nil); got.Code != http.StatusForbidden {
		t.Errorf("a person who runs work got %d", got.Code)
	}
	for _, target := range [][2]string{{"POST", "/admin/retention/sweep"}, {"POST", "/admin/retention/jobs/hold"}, {"POST", "/admin/retention/holds"}} {
		if got := here.as(t, "amy@acme.test").ask(t, target[0], target[1], url.Values{"who": {"x"}, "job": {"a-week-old"}}); got.Code != http.StatusForbidden {
			t.Errorf("%s %s gave a person who runs work %d", target[0], target[1], got.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(here.index.Root(), "a-week-old", "workspace")); err != nil {
		t.Error("a person who runs work swept a workspace")
	}
	here.signIn(t)
	page := here.ask(t, "GET", "/admin/retention", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("an admin got %d", page.Code)
	}
	for _, want := range []string{"2.0 KB", "a-week-old", "workspace", "7 days", "90 days", `href="/admin/retention"`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("the page lacks %q:\n%s", want, page.Body.String())
		}
	}
	plain := open(t)
	plain.signIn(t)
	if got := plain.ask(t, "GET", "/admin/retention", nil); got.Code != http.StatusNotFound {
		t.Errorf("a server that removes nothing gave %d", got.Code)
	}
}

func TestSweepingRemovesWhatIsDueAndTheChangeRecordSaysSo(t *testing.T) {
	here, log, dir := keeping(t)
	oldJob(t, here, "ten-days", 10)
	oldJob(t, here, "hundred-days", 100)
	oldJob(t, here, "one-day", 1)
	here.signIn(t)
	if got := here.ask(t, "POST", "/admin/retention/sweep", nil); got.Code != http.StatusSeeOther {
		t.Fatalf("sweeping gave %d", got.Code)
	}
	root := here.index.Root()
	if _, err := os.Stat(filepath.Join(root, "ten-days", "workspace")); err == nil {
		t.Error("a workspace past its time stayed")
	}
	if _, err := os.Stat(filepath.Join(root, "hundred-days")); err == nil {
		t.Error("a record past its time stayed")
	}
	if _, err := os.Stat(filepath.Join(root, "one-day", "workspace")); err != nil {
		t.Error("a young job lost its workspace")
	}
	swept := ofKind(written(t, log), changelog.Swept)
	if len(swept) != 1 || swept[0].What["workspaces"] != "1" || swept[0].What["records"] != "1" || swept[0].What["jobs"] != "hundred-days" {
		t.Fatalf("the sweep was not recorded as it happened: %+v", swept)
	}
	if _, err := os.Stat(filepath.Join(dir, lastSweepFile)); err != nil {
		t.Error("the last sweep was not kept")
	}
	if page := here.ask(t, "GET", "/admin/retention", nil).Body.String(); !strings.Contains(page, "The last sweep was") {
		t.Errorf("the page does not show the last sweep:\n%s", page)
	}
}

func TestAHoldKeepsAJobAndIsRecordedAndReleased(t *testing.T) {
	here, log, _ := keeping(t)
	oldJob(t, here, "held", 200)
	oldJob(t, here, "free", 200)
	here.signIn(t)

	if page := here.ask(t, "GET", "/jobs/held", nil).Body.String(); strings.Contains(page, "/hold") || strings.Contains(page, "Keep this job whole") {
		t.Fatalf("the job page carries a control that is the console's:\n%s", page)
	}
	if got := here.ask(t, "POST", "/admin/retention/jobs/hold", url.Values{"job": {"nothing-by-this-id"}}); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a job that is not there was held: %d %q", got.Code, got.Header().Get("Location"))
	}
	if got := here.ask(t, "POST", "/admin/retention/jobs/hold", url.Values{"job": {"held"}, "why": {"the audit"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("holding gave %d", got.Code)
	}
	if page := here.ask(t, "GET", "/admin/retention", nil).Body.String(); !strings.Contains(page, "the audit") || !strings.Contains(page, `action="/admin/retention/jobs/release"`) {
		t.Errorf("the retention page does not say the job is held:\n%s", page)
	}
	here.ask(t, "POST", "/admin/retention/sweep", nil)
	root := here.index.Root()
	if _, err := os.Stat(filepath.Join(root, "held", "workspace")); err != nil {
		t.Error("a held job was swept")
	}
	if _, found := here.index.Get("free"); found {
		t.Error("a job that was not held stayed")
	}

	here.ask(t, "POST", "/admin/retention/jobs/release", url.Values{"job": {"held"}})
	here.ask(t, "POST", "/admin/retention/sweep", nil)
	if _, found := here.index.Get("held"); found {
		t.Error("a released job was kept")
	}
	events := written(t, log)
	if len(ofKind(events, changelog.HoldSet)) != 1 || len(ofKind(events, changelog.HoldReleased)) != 1 {
		t.Fatalf("holds were not recorded: %+v", events)
	}
	if got := ofKind(events, changelog.HoldSet)[0]; got.What["job"] != "held" || got.What["why"] != "the audit" {
		t.Errorf("hold event %+v", got)
	}
}

func TestAHoldOnAPersonKeepsEverythingTheyAskedFor(t *testing.T) {
	here, _, _ := keeping(t)
	oldJob(t, here, "amys", 200)
	here.signIn(t)
	here.ask(t, "POST", "/admin/retention/holds", url.Values{"who": {"Amy@Acme.test"}, "why": {"legal"}})
	here.ask(t, "POST", "/admin/retention/sweep", nil)
	if _, found := here.index.Get("amys"); !found {
		t.Fatal("work of a held person was removed")
	}
	if page := here.ask(t, "GET", "/admin/retention", nil).Body.String(); !strings.Contains(page, "everything asked for by amy@acme.test") {
		t.Errorf("the hold is not listed:\n%s", page)
	}
	here.ask(t, "POST", "/admin/retention/holds/release", url.Values{"who": {"amy@acme.test"}})
	here.ask(t, "POST", "/admin/retention/sweep", nil)
	if _, found := here.index.Get("amys"); found {
		t.Error("a released person's work was kept")
	}
}

func TestASweepThatCannotReadItsHoldsRemovesNothing(t *testing.T) {
	here, _, dir := keeping(t)
	oldJob(t, here, "old", 200)
	if err := os.WriteFile(filepath.Join(dir, retention.HoldsFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	here.server.Sweep(time.Now())
	if _, found := here.index.Get("old"); !found {
		t.Fatal("a sweep guessed there were no holds and removed work")
	}
}
