package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/retention"
	"github.com/getlatentic/gusto/internal/tasks"
)

// keeping is a site that removes finished work: a working folder after 7 days and a
// record after 90, with the change log on.
func keeping(t *testing.T) (*site, *changelog.Log, string) {
	t.Helper()
	here, log := recording(t)
	dir := t.TempDir()
	here.server.UseRetention(dir, retention.Policy{
		WorkingFolderDays: map[string]int{"passed": 7}, RecordDays: map[string]int{"passed": 90},
	})
	return here, log, dir
}

// oldTask leaves a passed task that ended days ago, with a working folder.
func oldTask(t *testing.T, site *site, id string, days int) {
	t.Helper()
	spentTask(t, site.index.Root(), id, "", priced)
	work := filepath.Join(site.index.Root(), id, "workspace", "code.go")
	if err := os.MkdirAll(filepath.Dir(work), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(work, []byte(strings.Repeat("x", 2048)), 0o644); err != nil {
		t.Fatal(err)
	}
	ended := time.Now().AddDate(0, 0, -days).Unix()
	if err := site.index.Add(tasks.Record{ID: id, State: tasks.Passed, Brief: "brief " + id, Harness: "fixture",
		Asked: "amy@acme.test", Queued: ended, Started: ended, Ended: ended}); err != nil {
		t.Fatal(err)
	}
}

func TestTheRetentionPageIsAnAdminsAndOnlyWhereWorkIsRemoved(t *testing.T) {
	here, _, _ := keeping(t)
	oldTask(t, here, "a-week-old", 10)
	if got := here.as(t, "amy@acme.test").ask(t, "GET", "/admin/records?tab=keeping", nil); got.Code != http.StatusForbidden {
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
	if got := here.ask(t, "GET", "/admin/records?tab=keeping", nil); got.Code != http.StatusOK {
		t.Fatalf("an admin got %d", got.Code)
	}
	page := recordsPage(t, here, "?tab=keeping")
	kept := page.Keeping
	if kept == nil || !strings.Contains(page.tabHrefs(), keepingTab) {
		t.Fatalf("the keeping tab is not given: %+v", page)
	}
	if kept.WorkingFolders != "2.0 KB" || kept.Oldest != "a-week-old" || len(kept.Upcoming) != 1 || kept.Upcoming[0].Task != "a-week-old" || kept.Upcoming[0].Kind != "workspace" {
		t.Errorf("the keeping tab does not give the disk and the next sweep: %+v", kept)
	}
	if !kept.Removes || !slices.Contains(kept.Policy, retention.Row{State: "passed", WorkingFolder: 7, Record: 90}) {
		t.Errorf("the keeping tab does not give the policy: %+v", kept.Policy)
	}
	plain := open(t)
	plain.signIn(t)
	if got := plain.dataText(t, "/admin/records?tab=keeping"); strings.Contains(got, "tab=keeping") || strings.Contains(got, `"Keeping"`) {
		t.Errorf("a server that removes nothing offers keeping:\n%s", got)
	}
}

func TestSweepingRemovesWhatIsDueAndTheChangeRecordSaysSo(t *testing.T) {
	here, log, dir := keeping(t)
	oldTask(t, here, "ten-days", 10)
	oldTask(t, here, "hundred-days", 100)
	oldTask(t, here, "one-day", 1)
	here.signIn(t)
	if got := here.ask(t, "POST", "/admin/retention/sweep", nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/admin/records?tab=keeping" {
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
	if kept := recordsPage(t, here, "?tab=keeping").Keeping; kept.Last == nil || len(kept.Last.Actions) != 2 || kept.LastAt == "" {
		t.Errorf("the page does not give the last sweep: %+v", kept)
	}
}

func TestAHoldKeepsATaskAndIsRecordedAndReleased(t *testing.T) {
	here, log, _ := keeping(t)
	oldTask(t, here, "held", 200)
	oldTask(t, here, "free", 200)
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
	if held := recordsPage(t, here, "?tab=keeping").Keeping.HeldTasks; held["held"].Why != "the audit" || held["held"].By == "" {
		t.Errorf("the retention page does not say the job is held: %+v", held)
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
	oldTask(t, here, "amys", 200)
	here.signIn(t)
	here.ask(t, "POST", "/admin/retention/holds", url.Values{"who": {"Amy@Acme.test"}, "why": {"legal"}})
	here.ask(t, "POST", "/admin/retention/sweep", nil)
	if _, found := here.index.Get("amys"); !found {
		t.Fatal("work of a held person was removed")
	}
	if held := recordsPage(t, here, "?tab=keeping").Keeping.HeldPeople; held["amy@acme.test"].Why != "legal" {
		t.Errorf("the hold is not listed: %+v", held)
	}
	here.ask(t, "POST", "/admin/retention/holds/release", url.Values{"who": {"amy@acme.test"}})
	here.ask(t, "POST", "/admin/retention/sweep", nil)
	if _, found := here.index.Get("amys"); found {
		t.Error("a released person's work was kept")
	}
}

func TestASweepThatCannotReadItsHoldsRemovesNothing(t *testing.T) {
	here, _, dir := keeping(t)
	oldTask(t, here, "old", 200)
	if err := os.WriteFile(filepath.Join(dir, retention.HoldsFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	here.server.Sweep(time.Now())
	if _, found := here.index.Get("old"); !found {
		t.Fatal("a sweep guessed there were no holds and removed work")
	}
}
