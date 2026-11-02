package web

import (
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/integration"
	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/internal/retention"
	"github.com/getlatentic/gusto/internal/roster"
)

const boss = "boss@example.com"

// everything is a server with every section the console can have, and a boss
// who is an admin.
func everything(t *testing.T) *tokened {
	t.Helper()
	here := withTokens(t)
	here.server.UseRetention(t.TempDir(), retention.Policy{})
	list := filepath.Join(t.TempDir(), "members.json")
	if err := roster.Write(list, roster.Roster{}); err != nil {
		t.Fatal(err)
	}
	here.server.UseRoster(list)
	here.server.UseOrgSettings(filepath.Join(t.TempDir(), "settings.json"))
	here.server.UseAdmins([]string{boss})
	store, err := integration.Open(filepath.Join(t.TempDir(), "integrations"))
	if err != nil {
		t.Fatal(err)
	}
	here.server.setups.UseIntegrations(store)
	here.server.UseIntegrations(store, integration.Clone{Schemes: []string{"file"}})
	return here
}

var consoleReads = []string{
	"/admin", "/admin/spending", "/admin/activity.jsonl", "/admin/tokens", "/admin/people",
	"/admin/records", "/admin/records?tab=checks", "/admin/records?tab=keeping", "/admin/apps", "/admin/apps?tab=skills",
}

var consoleWrites = []string{
	"/admin/tokens", "/admin/tokens/abcdef12/revoke",
	"/admin/retention/sweep", "/admin/retention/holds", "/admin/retention/holds/release",
	"/admin/retention/jobs/hold", "/admin/retention/jobs/release",
	"/admin/skill-sets", "/admin/skill-sets/one/skills", "/admin/skill-sets/one/skills/two/drop",
	"/admin/budget",
	"/admin/integrations/import", "/admin/integrations/reports/enable", "/admin/integrations/reports/disable", "/admin/integrations/reports/restore",
}

func TestARunnerCannotReachTheConsoleAndAStrangerIsSentToSignIn(t *testing.T) {
	here := everything(t)
	runner, admin := here.as(t, alice), here.as(t, boss)
	tokens := len(here.server.tokens.List())
	form := url.Values{"name": {"sneaky"}, "scope": {"submit"}, "who": {"x"}, "job": {"x"}, "usd": {"1"}}

	for _, path := range consoleReads {
		if got := runner.ask(t, "GET", path, nil); got.Code != http.StatusForbidden {
			t.Errorf("a runner reading %s got %d", path, got.Code)
		}
		if got := admin.ask(t, "GET", path, nil); got.Code != http.StatusOK {
			t.Errorf("an admin reading %s got %d", path, got.Code)
		}
		if got := here.site.ask(t, "GET", path, nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/login" {
			t.Errorf("a stranger reading %s got %d to %q", path, got.Code, got.Header().Get("Location"))
		}
	}
	for _, path := range consoleWrites {
		if got := runner.ask(t, "POST", path, form); got.Code != http.StatusForbidden {
			t.Errorf("a runner posting to %s got %d", path, got.Code)
		}
	}
	if len(here.server.tokens.List()) != tokens {
		t.Error("a runner made a token")
	}
	if _, found := here.server.setups.Get("sneaky"); found {
		t.Error("a runner made a skill set")
	}
	if settings, err := here.server.orgSettings.Read(); err != nil || settings.MonthlyBudgetUSD != 0 {
		t.Errorf("a runner set the budget: %+v %v", settings, err)
	}
}

var navLink = regexp.MustCompile(`(?s)<nav [^>]*aria-label="Admin"[^>]*>(.*?)</nav>`)

func sectionsOf(t *testing.T, page string) string {
	t.Helper()
	found := navLink.FindStringSubmatch(page)
	if found == nil {
		t.Fatalf("the page has no console navigation:\n%s", page)
	}
	return found[1]
}

func TestTheConsoleListsASectionOnlyWhereTheServerHasIt(t *testing.T) {
	plain := open(t)
	plain.signIn(t)
	seen := sectionsOf(t, plain.ask(t, "GET", "/admin", nil).Body.String())
	for _, want := range []string{`href="/admin"`, `href="/admin/usage"`, `href="/admin/apps"`, `href="/admin/records"`} {
		if !strings.Contains(seen, want) {
			t.Errorf("a plain server does not list %s: %s", want, seen)
		}
	}
	for _, not := range []string{"/admin/people", "/admin/models", "/admin/spending"} {
		if strings.Contains(seen, not) {
			t.Errorf("a plain server lists %s, which it has nothing behind: %s", not, seen)
		}
	}
}

var sectionLink = regexp.MustCompile(`<a [^>]*href="(/admin[^"]*)"[^>]*>([^<]*)</a>`)

func TestAFullConsoleListsItsSectionsInOrder(t *testing.T) {
	full, _, _ := registered(t, registry.Rules{})
	admin := full.as(t, boss)
	seen := sectionsOf(t, admin.ask(t, "GET", "/admin", nil).Body.String())
	var got []string
	for _, link := range sectionLink.FindAllStringSubmatch(seen, -1) {
		got = append(got, link[2]+" "+link[1])
	}
	want := []string{"Overview /admin", "Usage /admin/usage", "Budgets /admin/budgets", "People /admin/people", "Models /admin/models", "Apps &amp; skills /admin/apps", "Records /admin/records"}
	if strings.Join(got, ", ") != strings.Join(want, ", ") {
		t.Errorf("the console lists\n%s\nand not\n%s", strings.Join(got, ", "), strings.Join(want, ", "))
	}
	for _, gone := range []string{"Verification", "Activity", "Retention", "Skill sets", "Integrations", "Connectors", "Tokens"} {
		if strings.Contains(seen, ">"+gone+"<") {
			t.Errorf("the console still lists %s", gone)
		}
	}
	if strings.Contains(seen, "Organization and billing") {
		t.Error("a console with no front door links to an organization page")
	}
}

// A page under a section marks that section, so a skill set's page is in Apps & skills and a tab of Records is in Records.
func TestTheConsoleMarksTheSectionAPageBelongsTo(t *testing.T) {
	admin := everything(t).as(t, boss)
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}})
	current := regexp.MustCompile(`href="([^"]+)" aria-current="page"`)
	for path, want := range map[string]string{
		"/admin/apps?tab=skills":     "/admin/apps",
		"/admin/skill-sets/reviewer": "/admin/apps",
		"/admin/records?tab=checks":  "/admin/records",
		"/admin/records?tab=keeping": "/admin/records",
		"/admin/people":              "/admin/people",
	} {
		marked := current.FindStringSubmatch(sectionsOf(t, admin.ask(t, "GET", path, nil).Body.String()))
		if marked == nil || marked[1] != want {
			t.Errorf("%s marks %v, not %s", path, marked, want)
		}
	}
}

// The pages that became tabs answer at their old addresses by sending an admin to the tab, with what they were sent to say.
func TestTheOldConsolePagesGoToTheirTabs(t *testing.T) {
	here := everything(t)
	admin, runner := here.as(t, boss), here.as(t, alice)
	for from, to := range map[string]string{
		"/admin/activity":                   "/admin/records?tab=activity",
		"/admin/verification":               "/admin/records?tab=checks",
		"/admin/retention":                  "/admin/records?tab=keeping",
		"/admin/connectors":                 "/admin/apps?tab=apps",
		"/admin/skill-sets":                 "/admin/apps?tab=skills",
		"/admin/integrations":               "/admin/apps?tab=skills",
		"/admin/skill-sets?refused=no+name": "/admin/apps?tab=skills&refused=no+name",
	} {
		got := admin.ask(t, "GET", from, nil)
		if got.Code != http.StatusSeeOther || got.Header().Get("Location") != to {
			t.Errorf("GET %s gave %d to %q", from, got.Code, got.Header().Get("Location"))
		}
		if got := runner.ask(t, "GET", from, nil); got.Code != http.StatusForbidden {
			t.Errorf("a runner asking for %s got %d", from, got.Code)
		}
	}
}

func TestEachTabOfAppsAndRecordsShowsItsOwnPart(t *testing.T) {
	admin := everything(t).as(t, boss)
	for path, want := range map[string][]string{
		"/admin/apps":                {"Apps &amp; skills", "What agents may use, for everyone in", "Add a skill", "Write a skill set", "Import from git", "Import an upload", `action="/admin/integrations/import"`},
		"/admin/records":             {"Records", "A signed record of everything", "Intact", " signed", "How to check it yourself", `href="/admin/activity.jsonl"`},
		"/admin/records?tab=checks":  {"Task checks", "Intact", "verified", "failed the check"},
		"/admin/records?tab=keeping": {"Keeping", "Intact", "Sweep now", `action="/admin/retention/holds"`, `action="/admin/retention/jobs/hold"`},
	} {
		page := admin.ask(t, "GET", path, nil)
		if page.Code != http.StatusOK {
			t.Fatalf("%s answered %d", path, page.Code)
		}
		for _, each := range want {
			if !strings.Contains(page.Body.String(), each) {
				t.Errorf("%s does not say %q", path, each)
			}
		}
	}
	if page := admin.ask(t, "GET", "/admin/records?tab=checks", nil).Body.String(); strings.Contains(page, "How to check it yourself") || strings.Contains(page, "Sweep now") {
		t.Error("the task checks tab carries another tab's part")
	}
}

func TestAnAppIsASwitchAndOneThatNeedsSetupSaysSo(t *testing.T) {
	here, _, _ := connected(t)
	page := here.ask(t, "GET", "/admin/apps", nil).Body.String()
	if !strings.Contains(page, `action="/admin/connectors/tasks/on"`) || !strings.Contains(page, `role="switch" aria-checked="false"`) {
		t.Fatalf("an app that is off is not a switch that turns it on:\n%s", page)
	}
	if strings.Contains(page, "Turn on</button>") {
		t.Error("the app is turned on with a button of words, not the switch")
	}
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	page = here.ask(t, "GET", "/admin/apps", nil).Body.String()
	if !strings.Contains(page, `action="/admin/connectors/tasks/off"`) || !strings.Contains(page, `role="switch" aria-checked="true"`) {
		t.Errorf("an app that is on is not a switch that turns it off:\n%s", page)
	}
	if !strings.Contains(page, "Agents use it only as that person, and only for what they ask.") {
		t.Error("the page does not say how agents use an app")
	}
}

func TestOnlyAnAdminSeesTheLinkToTheConsoleAndItIsTheOnlyOne(t *testing.T) {
	here := everything(t)
	runner, admin := here.as(t, alice), here.as(t, boss)
	oldJob(t, here.site, "seen", 1)
	where, _ := started(t, runner)
	pages := []string{"/chat", "/jobs", "/spend", "/presets", "/compare", "/setups", "/engines", "/jobs/seen", where}
	for _, path := range pages {
		as := admin.ask(t, "GET", path, nil).Body.String()
		if got := strings.Count(as, `href="/admin`); got != 1 || !strings.Contains(as, `href="/admin"`) {
			t.Errorf("%s gives an admin %d links to the console", path, got)
		}
		for _, control := range []string{`"/tokens"`, `"/retention"`, `"/changes"`, "/admin/"} {
			if strings.Contains(as, control) {
				t.Errorf("%s carries the admin control %s", path, control)
			}
		}
		if got := runner.ask(t, "GET", path, nil); strings.Contains(got.Body.String(), "/admin") {
			t.Errorf("%s shows a runner the console", path)
		}
	}
}

func TestTheConsoleHasOneLinkBack(t *testing.T) {
	admin := everything(t).as(t, boss)
	for _, path := range []string{"/admin", "/admin/spending", "/admin/tokens", "/admin/records", "/admin/records?tab=keeping", "/admin/apps?tab=skills", "/admin/people"} {
		page := admin.ask(t, "GET", path, nil).Body.String()
		if got := strings.Count(page, `href="/"`); got != 1 || !strings.Contains(page, "Back to gusto") {
			t.Errorf("%s has %d links to the employee's pages", path, got)
		}
		for _, employee := range []string{`href="/compare"`, `href="/jobs"`, `href="/presets"`, `href="/spend"`} {
			if strings.Contains(page, employee) {
				t.Errorf("%s carries the employee's navigation %s", path, employee)
			}
		}
	}
}

func TestTheAddressesTheAdminPagesHadStillWork(t *testing.T) {
	here := everything(t)
	admin, runner := here.as(t, boss), here.as(t, alice)
	for from, to := range map[string]string{
		"/changes": "/admin/records?tab=activity", "/changes.jsonl": "/admin/activity.jsonl",
		"/tokens": "/admin/tokens", "/retention": "/admin/records?tab=keeping",
	} {
		got := admin.ask(t, "GET", from, nil)
		if got.Code != http.StatusMovedPermanently || got.Header().Get("Location") != to {
			t.Errorf("GET %s gave %d to %q", from, got.Code, got.Header().Get("Location"))
		}
		if followed := admin.ask(t, "GET", to, nil); followed.Code != http.StatusOK {
			t.Errorf("%s does not answer an admin: %d", to, followed.Code)
		}
		if followed := runner.ask(t, "GET", got.Header().Get("Location"), nil); followed.Code != http.StatusForbidden {
			t.Errorf("a runner who followed %s got %d", from, followed.Code)
		}
		if stranger := here.site.ask(t, "GET", from, nil); stranger.Header().Get("Location") != "/login" {
			t.Errorf("a stranger asking for %s was sent to %q", from, stranger.Header().Get("Location"))
		}
	}
	for from, to := range map[string]string{
		"/tokens": "/admin/tokens", "/tokens/abcdef12/revoke": "/admin/tokens/abcdef12/revoke",
		"/retention/sweep": "/admin/retention/sweep", "/retention/holds": "/admin/retention/holds",
		"/retention/holds/release": "/admin/retention/holds/release",
		"/setups":                  "/admin/skill-sets", "/setups/one/skills": "/admin/skill-sets/one/skills",
		"/setups/one/skills/two/drop": "/admin/skill-sets/one/skills/two/drop",
	} {
		got := admin.ask(t, "POST", from, url.Values{"name": {"x"}})
		if got.Code != http.StatusPermanentRedirect || got.Header().Get("Location") != to {
			t.Errorf("POST %s gave %d to %q", from, got.Code, got.Header().Get("Location"))
		}
	}
	if _, found := here.server.setups.Get("x"); found {
		t.Error("a form sent to the old address made a skill set without going through the console")
	}
}

func TestTheConsoleNamesTheOrganizationAndAnotherOrganizationsTokenDoesNotOpenIt(t *testing.T) {
	here := behindAFrontDoor(t)
	here.server.UseAdmins(nil)
	stranger := here.enterWith(t, tokenFor(t, "other-com", "eve@other.com", "admin", time.Now()))
	if stranger.Code != http.StatusForbidden {
		t.Fatalf("a token for another organization was accepted: %d", stranger.Code)
	}
	if got := here.ask(t, "GET", "/admin", nil); got.Header().Get("Location") != "/login" {
		t.Errorf("without a sign-in of this organization the console answered %d", got.Code)
	}
	here.cookie = cookieOf(t, here.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	page := here.ask(t, "GET", "/admin", nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), ">acme-com</span>") {
		t.Errorf("the console does not say whose it is: %d\n%s", page.Code, page.Body.String())
	}
	org := here.server.front.page("/org", nil)
	if !strings.Contains(page.Body.String(), `href="`+html.EscapeString(org)+`">`) || !strings.Contains(page.Body.String(), "Organization and billing") {
		t.Errorf("the console does not link to the organization page %s", org)
	}
}
