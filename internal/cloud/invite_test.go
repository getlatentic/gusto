package cloud

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/computer"
)

// verifiedAdmin signs up an organization, follows the mail's link to confirm the
// admin's address, and returns the admin's browser.
func (f *front) verifiedAdmin(email, name string) *browser {
	f.t.Helper()
	b := f.browser()
	if status, _, _ := b.post("/signup", url.Values{"email": {email}, "org": {name}}); status != http.StatusSeeOther {
		f.t.Fatalf("sign-up: %d", status)
	}
	link := f.linkTo(email, "/verify?t=")
	if status, _, _ := b.post("/verify", url.Values{"t": {tokenOf(link)}}); status != http.StatusSeeOther {
		f.t.Fatalf("verify: %d", status)
	}
	return b
}

// linkTo is the link in the last mail to an address that begins with a path.
func (f *front) linkTo(email, begins string) string {
	f.t.Helper()
	mails := f.mails()
	for i := len(mails) - 1; i >= 0; i-- {
		if strings.Contains(mails[i], "To: "+email) && strings.Contains(mails[i], begins) {
			return linkInMail.FindStringSubmatch(mails[i])[1]
		}
	}
	f.t.Fatalf("no mail to %s with %s", email, begins)
	return ""
}

func tokenOf(link string) string { return link[strings.Index(link, "t=")+2:] }

func (f *front) invitedAndJoined(admin *browser, email string) *browser {
	f.t.Helper()
	if status, _, _ := admin.post("/org/invite", url.Values{"email": {email}}); status != http.StatusSeeOther {
		f.t.Fatalf("inviting %s: %d", email, status)
	}
	b := f.browser()
	if status, _, _ := b.post("/invite", url.Values{"t": {tokenOf(f.linkTo(email, "/invite?t="))}}); status != http.StatusSeeOther {
		f.t.Fatalf("joining as %s: %d", email, status)
	}
	return b
}

func TestAnAdminInvitesAPersonWhoFollowsTheLinkAndJoinsTheOrganization(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		if _, page := admin.get("/org"); !shown(t, page).CanInvite {
			t.Fatalf("a verified admin can invite: %+v", shown(t, page))
		}
		if status, _, header := admin.post("/org/invite", url.Values{"email": {"Bob@Acme.com"}}); status != http.StatusSeeOther || header.Get("Location") != "/org?sent=1" {
			t.Fatalf("%d %v", status, header)
		}
		if got := shownAt(t, admin, "/org"); !slices.Equal(got.Invited, []string{"bob@acme.com"}) || !got.IsAdmin {
			t.Errorf("the invitation is listed with a way to take it back: %+v", got)
		}
		link := f.linkTo("bob@acme.com", "/invite?t=")
		mail := f.mails()[len(f.mails())-1]
		if !strings.Contains(mail, "ada@acme.com invited you to join Acme") {
			t.Errorf("the mail says who invited them and to what:\n%s", mail)
		}

		bob := f.browser()
		for range 2 {
			status, page := bob.get(link)
			if got := shown(t, page); status != 200 || got.Page != "confirm" || got.Title != "Join Acme" || got.Inviter != "ada@acme.com" || got.Action != "/invite" || got.Token != tokenOf(link) {
				t.Fatalf("opening the link asks and does not answer: %d %+v", status, got)
			}
		}
		if _, found, _ := f.store.only(t.Context(), "bob@acme.com"); found {
			t.Fatal("opening the link made a person")
		}
		if status, _, header := bob.post("/invite", url.Values{"t": {tokenOf(link)}}); status != http.StatusSeeOther || header.Get("Location") != "/open" {
			t.Fatalf("%d %v", status, header)
		}
		got := shownAt(t, bob, "/org")
		if want := []memberView{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"}}; !slices.Equal(got.Members, want) || got.CanInvite || got.IsAdmin {
			t.Errorf("a runner sees the people, and invites and removes nobody: %+v", got)
		}
		if got := shownAt(t, admin, "/org"); !got.IsAdmin || len(got.Members) != 2 || len(got.Invited) != 0 {
			t.Errorf("the admin can remove him, and nothing is pending: %+v", got)
		}
		if status, _, _ := f.browser().post("/invite", url.Values{"t": {tokenOf(link)}}); status != http.StatusGone {
			t.Errorf("an invitation is answered once: %d", status)
		}
	})
}

func TestAnOrganizationThatHasNotConfirmedItsAdminCannotInvite(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		admin := f.browser()
		admin.post("/signup", url.Values{"email": {"ada@acme.com"}})
		if shownAt(t, admin, "/org").CanInvite {
			t.Error("an organization on the trial offers to invite")
		}
		status, page, _ := admin.post("/org/invite", url.Values{"email": {"bob@acme.com"}})
		if status != http.StatusConflict || !strings.Contains(shown(t, page).Notice, "cannot invite people until its admin has confirmed") {
			t.Errorf("%d %+v", status, shown(t, page))
		}
	})
}

func TestARunnerCannotInviteOrRemoveAndTheFormsOfOneOrganizationReachNoOther(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		ada := f.verifiedAdmin("ada@acme.com", "Acme")
		bob := f.invitedAndJoined(ada, "bob@acme.com")
		eve := f.verifiedAdmin("eve@globex.com", "Globex")
		f.invitedAndJoined(eve, "fay@globex.com")

		if status, _, _ := bob.post("/org/invite", url.Values{"email": {"cy@acme.com"}}); status != http.StatusForbidden {
			t.Errorf("a runner invites nobody: %d", status)
		}
		if status, _, _ := bob.post("/org/people/remove", url.Values{"email": {"ada@acme.com"}}); status != http.StatusForbidden {
			t.Errorf("a runner removes nobody: %d", status)
		}
		if status, _, _ := ada.post("/org/people/remove", url.Values{"email": {"fay@globex.com"}}); status != http.StatusUnprocessableEntity {
			t.Errorf("Acme's admin cannot remove Globex's person: %d", status)
		}
		if _, found, _ := f.store.only(t.Context(), "fay@globex.com"); !found {
			t.Error("a person of another organization was removed")
		}
		if _, page := ada.get("/org"); strings.Contains(page, "globex") || strings.Contains(page, "fay@") {
			t.Errorf("one organization's page names another's people:\n%s", page)
		}
	})
}

func TestARemovedPersonIsSignedOutOfGustoCloudAtOnce(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		ada := f.verifiedAdmin("ada@acme.com", "Acme")
		bob := f.invitedAndJoined(ada, "bob@acme.com")
		if status, _ := bob.get("/org"); status != 200 {
			t.Fatal(status)
		}
		if status, _, _ := ada.post("/org/people/remove", url.Values{"email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatal(status)
		}
		if status, _, header := bob.get2("/org"); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
			t.Errorf("the cookie they hold names nobody now: %d %v", status, header)
		}
		if status, _, header := bob.get2("/open"); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
			t.Errorf("and opens nothing: %d %v", status, header)
		}
		if status, _, _ := ada.post("/org/people/remove", url.Values{"email": {"ada@acme.com"}}); status != http.StatusConflict {
			t.Errorf("the last admin stays: %d", status)
		}
	})
}

func TestACancelledInvitationsLinkIsGoneAndTheOrganizationFillsAtAnAdminAndThree(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		ada := f.verifiedAdmin("ada@acme.com", "Acme")
		for _, who := range []string{"a", "b", "c"} {
			if status, _, _ := ada.post("/org/invite", url.Values{"email": {who + "@acme.com"}}); status != http.StatusSeeOther {
				t.Fatalf("%s: %d", who, status)
			}
		}
		if shownAt(t, ada, "/org").CanInvite {
			t.Error("a full organization offers to invite")
		}
		if status, page, _ := ada.post("/org/invite", url.Values{"email": {"d@acme.com"}}); status != http.StatusConflict || !strings.Contains(shown(t, page).Notice, "as many people as it may") {
			t.Errorf("%d %+v", status, shown(t, page))
		}
		link := f.linkTo("c@acme.com", "/invite?t=")
		if status, _, _ := ada.post("/org/invites/cancel", url.Values{"email": {"c@acme.com"}}); status != http.StatusSeeOther {
			t.Fatal(status)
		}
		if status, _ := f.browser().get(link); status != http.StatusGone {
			t.Errorf("a canceled invitation's link opens nothing: %d", status)
		}
		if !shownAt(t, ada, "/org").CanInvite {
			t.Error("a place was freed")
		}
	})
}

func TestSomeoneWhoJoinsIsSentToTheirOrganizationsServerWithTheRoleTheyHave(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServers(t)
		ada := f.signedUp("ada@acme.com")
		link := f.linkTo("ada@acme.com", "/verify?t=")
		ada.post("/verify", url.Values{"t": {tokenOf(link)}})
		bob := f.invitedAndJoined(ada, "bob@acme.com")
		status, _, header := bob.get2("/open")
		location := header.Get("Location")
		if status != http.StatusSeeOther || !strings.HasPrefix(location, "http://acme-com.cloud.test/auth/entry?t=") {
			t.Fatalf("%d %q", status, location)
		}
		parsed, _ := url.Parse(location)
		status, _, answer := bob.at(parsed.Host, "GET", parsed.RequestURI(), nil)
		if status != http.StatusSeeOther || !strings.Contains(strings.Join(answer.Values("Set-Cookie"), ";"), "gusto=bob@acme.com|runner") {
			t.Errorf("the organization's server has them as a runner: %d %v", status, answer)
		}
	})
}

func TestInvitationsAreLimitedForOneOrganizationInAnHour(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		ada := f.verifiedAdmin("ada@acme.com", "Acme")
		last := 0
		for range 21 {
			last, _, _ = ada.post("/org/invite", url.Values{"email": {"not an address"}})
		}
		if last != http.StatusTooManyRequests {
			t.Errorf("the twenty-first in an hour: %d", last)
		}
	})
}

func TestTheOrganizationsPageShowsWhichPeopleHaveComputersAndTheirState(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		ada := f.verifiedAdmin("ada@acme.com", "Acme")
		f.invitedAndJoined(ada, "bob@acme.com")
		registry, _ := f.store.Computers("acme-com")
		made, _, _ := registry.Ensure("bob@acme.com", "", "local")
		registry.Change(made.ID, func(c *computer.Computer) { c.State = computer.Running })
		got := shownAt(t, ada, "/org")
		if want := []memberView{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner", Computer: "running"}}; !slices.Equal(got.Members, want) || got.Computers != 1 || got.Quota.Computers != 4 {
			t.Errorf("Bob's computer and the count of them, and Ada has none: %+v", got)
		}
	})
}

// Join pressed a second time while the first press was on its way takes the person in, and the link is no use to anybody else.
func TestJoinPressedTwiceGoesInAndTheUsedLinkStaysUsedForOthers(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		admin.post("/org/invite", url.Values{"email": {"bob@acme.com"}})
		link := f.linkTo("bob@acme.com", "/invite?t=")
		bob := f.browser()
		if _, page := bob.get(link); shown(t, page).Page != "confirm" {
			t.Errorf("the link does not ask: %+v", shown(t, page))
		}
		for press := range 2 {
			if status, _, header := bob.post("/invite", url.Values{"t": {tokenOf(link)}}); status != http.StatusSeeOther || header.Get("Location") != "/open" {
				t.Fatalf("press %d: %d %v", press+1, status, header)
			}
		}
		if status, _, _ := f.browser().post("/invite", url.Values{"t": {tokenOf(link)}}); status != http.StatusGone {
			t.Errorf("another browser was let in by a used link: %d", status)
		}
	})
}
