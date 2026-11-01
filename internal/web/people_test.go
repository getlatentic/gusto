package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/roster"
)

// withPeople is a server behind a front door whose list of people is the one given,
// with an admin signed in.
func withPeople(t *testing.T, list roster.Roster) (*site, string) {
	t.Helper()
	here := behindAFrontDoor(t)
	path := filepath.Join(t.TempDir(), "members.json")
	if err := roster.Write(path, list); err != nil {
		t.Fatal(err)
	}
	here.server.UseRoster(path)
	here.cookie = cookieOf(t, here.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	return here, path
}

// spentNow leaves a job that somebody asked for and that cost this much just now.
func spentNow(t *testing.T, here *site, id, asked string, usd float64) {
	t.Helper()
	spentNowIn(t, here, id, asked, "", usd)
}

// spentNowIn is spentNow for work counted against a team.
func spentNowIn(t *testing.T, here *site, id, asked, team string, usd float64) {
	t.Helper()
	line := fmt.Sprintf(`{"path":"/v1/chat/completions","status":200,"atUnix":%d,"model":"m","inputTokens":10,"outputTokens":5,"usd":%v}`, time.Now().Unix(), usd)
	spentJob(t, here.index.Root(), id, team, line)
	if err := here.index.Add(jobs.Record{ID: id, State: jobs.Passed, Brief: id, Harness: "fixture", Asked: asked, Queued: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
}

// unmeasuredNow is a call with no count and no cost that was sent this moment, which a test about this month needs: a
// call on a fixed date is in this month only until the month ends.
func unmeasuredNow() string {
	return fmt.Sprintf(`{"path":"/v1/chat/completions","status":200,"atUnix":%d,"model":"m","inputTokens":null,"outputTokens":null,"usd":null}`, time.Now().Unix())
}

func rowOf(t *testing.T, page, email string) string {
	t.Helper()
	found := regexp.MustCompile(`(?s)<tr>\s*<td>(?:<span class="[^"]*">.*?<span class="text-xs text-muted">)?` + regexp.QuoteMeta(email) + `(?:</span></span></span>)?</td>(.*?)</tr>`).FindStringSubmatch(page)
	if found == nil {
		t.Fatalf("%s is not in the table:\n%s", email, page)
	}
	return found[1]
}

func TestThePeoplePageListsMembersWithRoleLastSignInAndWhatTheySpent(t *testing.T) {
	now := time.Now()
	here, _ := withPeople(t, roster.Roster{
		Written: now,
		Members: []roster.Member{
			{Email: "ada@acme.com", Role: "admin", Joined: now.Add(-72 * time.Hour), LastSignIn: now.Add(-2 * time.Hour)},
			{Email: "bob@acme.com", Role: "runner", Joined: now.Add(-24 * time.Hour)},
			{Email: "cy@acme.com", Role: "runner", Joined: now.Add(-24 * time.Hour), LastSignIn: now.Add(-3 * 24 * time.Hour)},
		},
		Invited: []roster.Invited{{Email: "dee@acme.com", By: "ada@acme.com", Created: now, Expires: now.Add(7 * 24 * time.Hour)}},
	})
	spentNow(t, here, "ada-1", "ada@acme.com", 0.25)
	spentNow(t, here, "ada-2", "ada@acme.com", 0.5)
	spentNow(t, here, "cy-1", "cy@acme.com", 2)
	spentNow(t, here, "token-1", "token:ci", 9)

	answer := here.ask(t, "GET", "/admin/people", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("%d", answer.Code)
	}
	page := answer.Body.String()
	for email, wants := range map[string][]string{
		"ada@acme.com": {"admin", "2h ago", "$0.7500"},
		"bob@acme.com": {"runner", "never", "nothing"},
		"cy@acme.com":  {"runner", "$2.00"},
	} {
		row := rowOf(t, page, email)
		for _, want := range wants {
			if !strings.Contains(row, want) {
				t.Errorf("the row of %s lacks %q: %s", email, want, row)
			}
		}
	}
	if strings.Contains(rowOf(t, page, "bob@acme.com"), "$") || strings.Contains(page, "$9") {
		t.Errorf("what a token spent was put against a person:\n%s", page)
	}
	for _, want := range []string{"dee@acme.com", "Invited", `href="https://cloud.test/org?org=acme-com"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q:\n%s", want, page)
		}
	}
}

func TestAPersonsSpendIsFoundWhateverTheCaseTheirAddressWasRecordedIn(t *testing.T) {
	here, _ := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}})
	spentNow(t, here, "ada-1", "Ada@Acme.com", 1)
	if row := rowOf(t, here.ask(t, "GET", "/admin/people", nil).Body.String(), "ada@acme.com"); !strings.Contains(row, "$1.00") {
		t.Errorf("the spend of an address in another case is missing: %s", row)
	}
}

func TestWhenTheListOfPeopleCannotBeReadThePageSaysSoAndShowsNobody(t *testing.T) {
	here, path := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}})
	for name, damage := range map[string]func(){
		"it is not there":  func() { os.Remove(path) },
		"it is not a list": func() { os.WriteFile(path, []byte("{this is not json"), 0o600) },
	} {
		damage()
		page := here.ask(t, "GET", "/admin/people", nil)
		body := page.Body.String()
		if page.Code != http.StatusOK || !strings.Contains(body, "could not be read") {
			t.Errorf("%s: the page does not say so: %d\n%s", name, page.Code, body)
		}
		if strings.Contains(body, "ada@acme.com") || strings.Contains(body, "<table") {
			t.Errorf("%s: the page shows people it has no list of:\n%s", name, body)
		}
	}
	if err := roster.Write(path, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}}); err != nil {
		t.Fatal(err)
	}
	if body := here.ask(t, "GET", "/admin/people", nil).Body.String(); !strings.Contains(body, "ada@acme.com") {
		t.Error("a list that was written again is not shown")
	}
}

func TestThereIsNoPeoplePageWhereNoFrontDoorKeepsThePeople(t *testing.T) {
	here := open(t)
	here.signIn(t)
	if got := here.ask(t, "GET", "/admin/people", nil); got.Code != http.StatusNotFound {
		t.Errorf("a server with no list of people answered %d", got.Code)
	}
	if strings.Contains(here.ask(t, "GET", "/admin", nil).Body.String(), "/admin/people") {
		t.Error("the console lists people where there are none to list")
	}
}

// Behind a front door, which owns roles, a role is a choice sent there, and it comes back to this page.
func TestARoleIsChangedAtTheFrontDoorAndComesBackToPeople(t *testing.T) {
	here, _ := withPeople(t, roster.Roster{Members: []roster.Member{
		{Email: "ada@acme.com", Role: "admin"}, {Email: "bea@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"},
	}})
	page := here.ask(t, "GET", "/admin/people", nil).Body.String()
	for _, email := range []string{"ada@acme.com", "bea@acme.com", "bob@acme.com"} {
		row := rowOf(t, page, email)
		for _, want := range []string{`action="https://cloud.test/org/people/role?org=acme-com"`, `name="email" value="` + email + `"`,
			`name="back" value="http://example.com/admin/people"`, `name="role"`, "data-autosubmit"} {
			if !strings.Contains(row, want) {
				t.Errorf("the role of %s lacks %q:\n%s", email, want, row)
			}
		}
	}
	if row := rowOf(t, page, "bea@acme.com"); !strings.Contains(row, `data-confirm-when="runner"`) || !strings.Contains(row, `<option value="admin" selected>`) {
		t.Errorf("making an admin a runner does not ask first:\n%s", row)
	}
	if row := rowOf(t, page, "bob@acme.com"); strings.Contains(row, "data-confirm") || !strings.Contains(row, `<option value="runner" selected>`) {
		t.Errorf("making a runner an admin asks, or the runner is not shown as one:\n%s", row)
	}
}

func TestTheOnlyAdminAndAServerWithNoFrontDoorShowTheRoleInWords(t *testing.T) {
	here, _ := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"}}})
	page := here.ask(t, "GET", "/admin/people", nil).Body.String()
	if row := rowOf(t, page, "ada@acme.com"); strings.Contains(row, "<select") || !strings.Contains(row, ">admin</span>") {
		t.Errorf("the only admin is offered to stop being one:\n%s", row)
	}
	if row := rowOf(t, page, "bob@acme.com"); !strings.Contains(row, "<select") {
		t.Errorf("a runner is not offered to be made an admin:\n%s", row)
	}

	alone, _, _ := budgeted(t)
	if page := people(t, alone); strings.Contains(page, "<select") || strings.Contains(page, "/org/people/role") || !strings.Contains(page, ">runner</span>") {
		t.Errorf("a server with no front door offers to change a role:\n%s", page)
	}
}
