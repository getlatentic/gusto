package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/tasks"
)

// withPeople is a server behind gusto cloud whose list of people is the one given,
// with an admin signed in.
func withPeople(t *testing.T, list roster.Roster) (*site, string) {
	t.Helper()
	here := behindCloud(t)
	path := filepath.Join(t.TempDir(), "members.json")
	if err := roster.Write(path, list); err != nil {
		t.Fatal(err)
	}
	here.server.UseRoster(path)
	here.cookie = cookieOf(t, here.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	return here, path
}

// spentNow leaves a task that somebody asked for and that cost this much just now.
func spentNow(t *testing.T, here *site, id, asked string, usd float64) {
	t.Helper()
	spentNowIn(t, here, id, asked, "", usd)
}

// spentNowIn is spentNow for work counted against a team.
func spentNowIn(t *testing.T, here *site, id, asked, team string, usd float64) {
	t.Helper()
	line := fmt.Sprintf(`{"path":"/v1/chat/completions","status":200,"atUnix":%d,"model":"m","inputTokens":10,"outputTokens":5,"usd":%v}`, time.Now().Unix(), usd)
	spentTaskFor(t, here.index.Root(), id, team, asked, line)
	if err := here.index.Add(tasks.Record{ID: id, State: tasks.Passed, Brief: id, Harness: "fixture", Asked: asked, Queued: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
}

// unmeasuredNow is a call with no count and no cost that was sent this moment, which a test about this month needs: a
// call on a fixed date is in this month only until the month ends.
func unmeasuredNow() string {
	return fmt.Sprintf(`{"path":"/v1/chat/completions","status":200,"atUnix":%d,"model":"m","inputTokens":null,"outputTokens":null,"usd":null}`, time.Now().Unix())
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

	page := people(t, here, "")
	ada, bob, cy := personIn(t, page, "ada@acme.com"), personIn(t, page, "bob@acme.com"), personIn(t, page, "cy@acme.com")
	if ada.Role != "admin" || ada.LastSignIn != now.Add(-2*time.Hour).Unix() || ada.spentOf() != 0.75 {
		t.Errorf("ada's row is %+v", ada)
	}
	if bob.Role != "runner" || bob.LastSignIn != 0 || bob.Spent != nil {
		t.Errorf("bob, who never signed in and spent nothing, is %+v", bob)
	}
	if cy.Role != "runner" || cy.spentOf() != 2 {
		t.Errorf("cy's row is %+v", cy)
	}
	if strings.Contains(here.dataText(t, "/admin/people"), "token:ci") {
		t.Error("what a token spent was put against a person")
	}
	if len(page.Tabs) != 2 || page.Tabs[1].Href != "/admin/people?tab=invited" || page.Tabs[1].Count != 1 {
		t.Errorf("People has no Invited tab with one invitation: %+v", page.Tabs)
	}
	if page.Manage != "https://cloud.test/org?org=acme-com" {
		t.Errorf("People invites people at %q", page.Manage)
	}
	for _, each := range page.People {
		if each.Email == "dee@acme.com" {
			t.Error("the People tab lists who is only invited")
		}
	}
	invited := people(t, here, "?tab=invited")
	if invited.Tab != "invited" || len(invited.Invited) != 1 || invited.Invited[0].Email != "dee@acme.com" || len(invited.People) != 0 {
		t.Errorf("the Invited tab does not list only who is invited: %+v", invited)
	}
}

func TestAPersonsSpendIsFoundWhateverTheCaseTheirAddressWasRecordedIn(t *testing.T) {
	here, _ := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}})
	spentNow(t, here, "ada-1", "Ada@Acme.com", 1)
	if ada := personIn(t, people(t, here, ""), "ada@acme.com"); ada.spentOf() != 1 {
		t.Errorf("the spend of an address in another case is missing: %+v", ada)
	}
}

func TestWhenTheListOfPeopleCannotBeReadThePageSaysSoAndShowsNobody(t *testing.T) {
	here, path := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}})
	// The list was read once, as every request reads it, so ada stays signed in on what it said.
	people(t, here, "")
	for name, damage := range map[string]func(){
		"it is not there":  func() { os.Remove(path) },
		"it is not a list": func() { os.WriteFile(path, []byte("{this is not json"), 0o600) },
	} {
		damage()
		page := people(t, here, "")
		if page.Unreadable == "" || len(page.People) != 0 {
			t.Errorf("%s: the page does not say so, or shows people it has no list of: %+v", name, page)
		}
	}
	if err := roster.Write(path, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}}}); err != nil {
		t.Fatal(err)
	}
	if page := people(t, here, ""); len(page.People) != 1 {
		t.Error("a list that was written again is not shown")
	}
}

func TestThereIsNoPeoplePageWhereNoGustoCloudKeepsThePeople(t *testing.T) {
	here := open(t)
	here.signIn(t)
	for _, path := range []string{"/admin/people", "/app/api/admin/people"} {
		if got := here.ask(t, "GET", path, nil); got.Code != http.StatusNotFound {
			t.Errorf("a server with no list of people answered %d at %s", got.Code, path)
		}
	}
	if strings.Contains(here.ask(t, "GET", "/app/api/console", nil).Body.String(), "/admin/people") {
		t.Error("the console lists people where there are none to list")
	}
}

// Behind gusto cloud, which owns roles, a role is a choice sent there, and it comes back to this page.
func TestARoleIsChangedAtGustoCloudAndComesBackToPeople(t *testing.T) {
	here, _ := withPeople(t, roster.Roster{Members: []roster.Member{
		{Email: "ada@acme.com", Role: "admin"}, {Email: "bea@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"},
	}})
	page := people(t, here, "")
	if page.RoleAction != "https://cloud.test/org/people/role?org=acme-com" || page.Back != "http://example.com/admin/people" {
		t.Errorf("a role is not sent to gusto cloud and back: %q %q", page.RoleAction, page.Back)
	}
	if page.Admins != 2 || personIn(t, page, "bea@acme.com").Role != "admin" || personIn(t, page, "bob@acme.com").Role != "runner" {
		t.Errorf("the roles are not what the list says: %+v", page)
	}
}

func TestTheOnlyAdminAndAServerWithNoGustoCloudShowTheRoleInWords(t *testing.T) {
	here, _ := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"}}})
	if page := people(t, here, ""); page.Admins != 1 || page.RoleAction == "" {
		t.Errorf("the page does not say there is one admin to keep: %+v", page)
	}
	alone, _, _ := budgeted(t)
	if page := people(t, alone, ""); page.RoleAction != "" || page.Back != "" {
		t.Errorf("a server with no gusto cloud offers to change a role: %+v", page)
	}
}
