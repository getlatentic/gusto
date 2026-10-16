package cloud

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/inbox"
	"github.com/getlatentic/gusto/internal/roster"
)

// acme is an organisation with a verified admin, ada, and a front door that starts its
// server, which is the fake.
func acme(t *testing.T) (*front, *browser) {
	t.Helper()
	f := withServers(t)
	ada := f.signedUp("ada@acme.com")
	if status, _, _ := ada.post("/verify", url.Values{"t": {tokenOf(f.linkTo("ada@acme.com", "/verify?t="))}}); status != http.StatusSeeOther {
		t.Fatalf("verifying: %d", status)
	}
	return f, ada
}

func (f *front) roster() roster.Roster {
	f.t.Helper()
	list, err := roster.Read(f.store.RosterFile("acme-com"))
	if err != nil {
		f.t.Fatal(err)
	}
	return list
}

func (f *front) events() []inbox.Waiting {
	f.t.Helper()
	waiting, err := inbox.Pending(f.store.InboxDir("acme-com"))
	if err != nil {
		f.t.Fatal(err)
	}
	return waiting
}

func member(list roster.Roster, email string) (roster.Member, bool) {
	for _, each := range list.Members {
		if each.Email == email {
			return each, true
		}
	}
	return roster.Member{}, false
}

func TestTheServerIsGivenTheListOfPeopleAtStartAndAfterEveryChangeToIt(t *testing.T) {
	f, ada := acme(t)
	if got, found := member(f.roster(), "ada@acme.com"); !found || got.Role != "admin" || !got.LastSignIn.IsZero() {
		t.Fatalf("at start the list has the admin, who has not been signed in: %+v", f.roster())
	}

	ada.post("/org/invite", url.Values{"email": {"Bob@Acme.com"}})
	if list := f.roster(); len(list.Invited) != 1 || list.Invited[0].Email != "bob@acme.com" || list.Invited[0].By != "ada@acme.com" || len(list.Members) != 1 {
		t.Errorf("an invitation is in the list, and is not a member: %+v", list)
	}

	bob := f.browser()
	bob.post("/invite", url.Values{"t": {tokenOf(f.linkTo("bob@acme.com", "/invite?t="))}})
	list := f.roster()
	if got, found := member(list, "bob@acme.com"); !found || got.Role != "runner" || len(list.Invited) != 0 {
		t.Errorf("a person who joined is a runner, and is no longer invited: %+v", list)
	}

	ada.post("/org/people/role", url.Values{"email": {"bob@acme.com"}, "role": {"admin"}})
	if got, _ := member(f.roster(), "bob@acme.com"); got.Role != "admin" {
		t.Errorf("a change of role is in the list: %+v", got)
	}

	before := time.Now().Add(-time.Minute)
	if status, _, _ := ada.get2("/open"); status != http.StatusSeeOther {
		t.Fatalf("opening: %d", status)
	}
	list = f.roster()
	if got, _ := member(list, "ada@acme.com"); got.LastSignIn.Before(before) {
		t.Errorf("signing in to the organisation is in the list: %+v", got)
	}
	if got, _ := member(list, "bob@acme.com"); !got.LastSignIn.IsZero() {
		t.Errorf("a person who was not signed in has no sign-in: %+v", got)
	}

	ada.post("/org/invite", url.Values{"email": {"cy@acme.com"}})
	ada.post("/org/invites/cancel", url.Values{"email": {"cy@acme.com"}})
	if list := f.roster(); len(list.Invited) != 0 {
		t.Errorf("a cancelled invitation is still listed: %+v", list.Invited)
	}

	ada.post("/org/people/remove", url.Values{"email": {"bob@acme.com"}})
	if _, found := member(f.roster(), "bob@acme.com"); found {
		t.Error("a removed person is still in the list")
	}
}

func TestOpeningAnAppIsNotSigningInToTheOrganisation(t *testing.T) {
	f, ada := acme(t)
	if status, _, _ := ada.get2("/open?app=abcdef0123"); status != http.StatusSeeOther {
		t.Fatalf("opening an app: %d", status)
	}
	if got, _ := member(f.roster(), "ada@acme.com"); !got.LastSignIn.IsZero() {
		t.Errorf("opening an app was counted as signing in: %+v", got)
	}
}

func TestAnAdminChangesRolesAndAnOrganisationKeepsAnAdmin(t *testing.T) {
	f, ada := acme(t)
	bob := f.invitedAndJoined(ada, "bob@acme.com")
	f.signedUp("eve@globex.com")

	role := func(who *browser, email, role string) int {
		status, _, _ := who.post("/org/people/role", url.Values{"email": {email}, "role": {role}})
		return status
	}
	roleOf := func(email string) Role {
		person, _, _ := f.store.Person(email)
		return person.Role
	}

	if status := role(bob, "bob@acme.com", "admin"); status != http.StatusForbidden || roleOf("bob@acme.com") != Runner {
		t.Errorf("a runner made themselves an admin: %d", status)
	}
	if status := role(ada, "bob@acme.com", "admin"); status != http.StatusSeeOther || roleOf("bob@acme.com") != Admin {
		t.Errorf("an admin makes a runner an admin: %d", status)
	}
	if status := role(ada, "bob@acme.com", "runner"); status != http.StatusSeeOther || roleOf("bob@acme.com") != Runner {
		t.Errorf("and a runner again: %d", status)
	}
	if status := role(ada, "ada@acme.com", "runner"); status != http.StatusConflict || roleOf("ada@acme.com") != Admin {
		t.Errorf("the last admin stays an admin: %d", status)
	}
	role(ada, "bob@acme.com", "admin")
	if status := role(ada, "ada@acme.com", "runner"); status != http.StatusSeeOther || roleOf("ada@acme.com") != Runner {
		t.Errorf("with another admin there, an admin may step down: %d", status)
	}
	if status := role(bob, "bob@acme.com", "runner"); status != http.StatusConflict || roleOf("bob@acme.com") != Admin {
		t.Errorf("and the one left is the last: %d", status)
	}
	if status := role(bob, "ada@acme.com", "owner"); status != http.StatusUnprocessableEntity || roleOf("ada@acme.com") != Runner {
		t.Errorf("a role that is neither of the two: %d", status)
	}
	if status := role(bob, "nobody@acme.com", "admin"); status != http.StatusUnprocessableEntity {
		t.Errorf("nobody by that address: %d", status)
	}
	if status := role(bob, "eve@globex.com", "runner"); status != http.StatusUnprocessableEntity || roleOf("eve@globex.com") != Admin {
		t.Errorf("a person of another organisation is not this one's to change: %d", status)
	}
	if got, _ := member(f.roster(), "ada@acme.com"); got.Role != "runner" {
		t.Errorf("the list follows: %+v", got)
	}
}

func TestInvitingChangingARoleAndRemovingArePassedToTheServerForItsRecord(t *testing.T) {
	f, ada := acme(t)
	ada.post("/org/invite", url.Values{"email": {"bob@acme.com"}})
	bob := f.browser()
	bob.post("/invite", url.Values{"t": {tokenOf(f.linkTo("bob@acme.com", "/invite?t="))}})
	ada.post("/org/people/role", url.Values{"email": {"bob@acme.com"}, "role": {"admin"}})
	ada.post("/org/people/remove", url.Values{"email": {"bob@acme.com"}})

	got := f.events()
	if len(got) != 3 {
		t.Fatalf("three changes were made: %+v", got)
	}
	want := []struct {
		kind string
		what map[string]string
	}{
		{changelog.MemberInvited, map[string]string{"email": "bob@acme.com", "role": "runner"}},
		{changelog.MemberRole, map[string]string{"email": "bob@acme.com", "was": "runner", "now": "admin"}},
		{changelog.MemberRemoved, map[string]string{"email": "bob@acme.com", "role": "admin"}},
	}
	for i, each := range want {
		event := got[i].Item
		if got[i].Err != nil || event.Kind != each.kind || event.Who != "ada@acme.com" || event.Role != "admin" || event.At.IsZero() || event.From != "127.0.0.1" {
			t.Errorf("event %d: %+v", i, got[i])
		}
		for key, value := range each.what {
			if event.What[key] != value {
				t.Errorf("event %d (%s) says %s is %q, not %q", i, each.kind, key, event.What[key], value)
			}
		}
	}
}

func TestWhatIsRefusedOrChangesNothingIsNotRecorded(t *testing.T) {
	f, ada := acme(t)
	bob := f.invitedAndJoined(ada, "bob@acme.com")
	drain := len(f.events())

	bob.post("/org/invite", url.Values{"email": {"cy@acme.com"}})
	bob.post("/org/people/role", url.Values{"email": {"bob@acme.com"}, "role": {"admin"}})
	bob.post("/org/people/remove", url.Values{"email": {"ada@acme.com"}})
	ada.post("/org/people/remove", url.Values{"email": {"ada@acme.com"}})
	ada.post("/org/people/role", url.Values{"email": {"ada@acme.com"}, "role": {"runner"}})
	for _, same := range []url.Values{{"email": {"ada@acme.com"}, "role": {"admin"}}, {"email": {"bob@acme.com"}, "role": {"runner"}}} {
		if status, _, _ := ada.post("/org/people/role", same); status != http.StatusSeeOther {
			t.Errorf("asking for the role a person has is not a refusal: %d", status)
		}
	}
	ada.post("/org/invite", url.Values{"email": {"ada@acme.com"}})
	ada.post("/org/invite", url.Values{"email": {"not an address"}})

	if got := f.events(); len(got) != drain {
		t.Errorf("changes that were refused or did nothing were passed on: %+v", got[drain:])
	}
}

func TestAnAdminIsOfferedTheOtherRoleAndTheLastAdminIsNotOfferedRunner(t *testing.T) {
	f, ada := acme(t)
	bob := f.invitedAndJoined(ada, "bob@acme.com")
	_, page := ada.get("/org")
	if !strings.Contains(page, `name="role" value="admin"`) {
		t.Errorf("an admin cannot make a runner an admin:\n%s", page)
	}
	if strings.Contains(page, `name="role" value="runner"`) {
		t.Errorf("the only admin is offered to become a runner:\n%s", page)
	}
	if _, page := bob.get("/org"); strings.Contains(page, "/org/people/role") {
		t.Errorf("a runner is offered to change roles:\n%s", page)
	}
	ada.post("/org/people/role", url.Values{"email": {"bob@acme.com"}, "role": {"admin"}})
	if _, page := ada.get("/org"); !strings.Contains(page, `name="role" value="runner"`) {
		t.Errorf("with two admins either may step down:\n%s", page)
	}
}
