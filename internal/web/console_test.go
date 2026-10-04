package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/retention"
	"github.com/getlatentic/haas/internal/roster"
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
	return here
}

var consoleReads = []string{
	"/admin", "/admin/spending", "/admin/activity", "/admin/activity.jsonl",
	"/admin/tokens", "/admin/retention", "/admin/skill-sets", "/admin/people",
}

var consoleWrites = []string{
	"/admin/tokens", "/admin/tokens/abcdef12/revoke",
	"/admin/retention/sweep", "/admin/retention/holds", "/admin/retention/holds/release",
	"/admin/retention/jobs/hold", "/admin/retention/jobs/release",
	"/admin/skill-sets", "/admin/skill-sets/one/skills", "/admin/skill-sets/one/skills/two/drop",
	"/admin/budget",
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

var navLink = regexp.MustCompile(`(?s)<nav class="sections"[^>]*>(.*?)</nav>`)

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
	for _, want := range []string{`href="/admin"`, `href="/admin/spending"`, `href="/admin/skill-sets"`} {
		if !strings.Contains(seen, want) {
			t.Errorf("a plain server does not list %s: %s", want, seen)
		}
	}
	for _, not := range []string{"/admin/activity", "/admin/tokens", "/admin/retention", "/admin/people"} {
		if strings.Contains(seen, not) {
			t.Errorf("a plain server lists %s, which it has nothing behind: %s", not, seen)
		}
	}

	full := everything(t).as(t, boss)
	seen = sectionsOf(t, full.ask(t, "GET", "/admin", nil).Body.String())
	for _, want := range []string{"/admin/activity", "/admin/tokens", "/admin/retention", "/admin/skill-sets", "/admin/spending", "/admin/people"} {
		if !strings.Contains(seen, want) {
			t.Errorf("a full server does not list %s: %s", want, seen)
		}
	}
	if strings.Contains(seen, "Policy") {
		t.Errorf("the console lists a section that has no page: %s", seen)
	}
}

func TestOnlyAnAdminSeesTheLinkToTheConsoleAndItIsTheOnlyOne(t *testing.T) {
	here := everything(t)
	runner, admin := here.as(t, alice), here.as(t, boss)
	oldJob(t, here.site, "seen", 1)
	where, _ := started(t, runner)
	pages := []string{"/", "/jobs", "/spend", "/new", "/compare", "/setups", "/engines", "/jobs/seen", where}
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
	for _, path := range []string{"/admin", "/admin/spending", "/admin/tokens", "/admin/retention", "/admin/skill-sets", "/admin/activity", "/admin/people"} {
		page := admin.ask(t, "GET", path, nil).Body.String()
		if got := strings.Count(page, `href="/"`); got != 1 || !strings.Contains(page, "Back to Latentic Agent Cloud") {
			t.Errorf("%s has %d links to the employee's pages", path, got)
		}
		for _, employee := range []string{`href="/compare"`, `href="/jobs"`, `href="/new"`, `href="/spend"`} {
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
		"/changes": "/admin/activity", "/changes.jsonl": "/admin/activity.jsonl",
		"/tokens": "/admin/tokens", "/retention": "/admin/retention",
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

func TestTheConsoleNamesTheOrganisationAndAnotherOrganisationsTokenDoesNotOpenIt(t *testing.T) {
	here := behindAFrontDoor(t)
	here.server.UseAdmins(nil)
	stranger := here.enterWith(t, tokenFor(t, "other-com", "eve@other.com", "admin", time.Now()))
	if stranger.Code != http.StatusForbidden {
		t.Fatalf("a token for another organisation was accepted: %d", stranger.Code)
	}
	if got := here.ask(t, "GET", "/admin", nil); got.Header().Get("Location") != "/login" {
		t.Errorf("without a sign-in of this organisation the console answered %d", got.Code)
	}
	here.cookie = cookieOf(t, here.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	page := here.ask(t, "GET", "/admin", nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Admin · acme-com") {
		t.Errorf("the console does not say whose it is: %d\n%s", page.Code, page.Body.String())
	}
}
