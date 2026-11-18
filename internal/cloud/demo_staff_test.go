package cloud

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// staffSet is a host whose demos are a sales call: only its team may start one, and visitors sign up.
func staffSet() Settings {
	set := demoSet
	set.Demo.Staff = []string{"latentic.com", "sales@partner.com"}
	return set
}

func TestOnlyTheStaffMayStartADemoWhereStaffAreNamed(t *testing.T) {
	staff := staffSet().Demo
	for address, may := range map[string]bool{
		"ada@latentic.com": true, "Ada@Latentic.COM": true, "sales@partner.com": true,
		"ops@partner.com": false, "ada@notlatentic.com": false, "ada@latentic.com.evil": false, "latentic.com": false, "": false,
	} {
		if staff.MayStart(address) != may {
			t.Errorf("%q may start a demo: %v, and should: %v", address, !may, may)
		}
	}
	if !demoSet.Demo.MayStart("anyone@example.com") || !demoSet.Demo.Public() || staff.Public() {
		t.Error("a host that names no staff gives demos to anyone, and only that one")
	}
}

// Where demos are given by the team, the landing page leads to sign-up and to booking one, and says what an organization starts
// with; /demo offers to book one, and the form is there for the team.
func TestWhereOnlyTheStaffGiveDemosTheLandingPageLeadsToSignUp(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		status, page, _ := landingAt(t, staffSet())
		if got := shown(t, page); status != http.StatusOK || got.Page != "landing" || got.Book != "https://cal.example/gusto" {
			t.Errorf("%d: the landing page does not lead to sign-up and booking a demo: %+v", status, got)
		}

		f := newFront(t, staffSet(), nil)
		status, page = f.browser().get("/demo")
		if got := shown(t, page); status != http.StatusOK || got.Page != "demo-book" || got.Book != "https://cal.example/gusto" {
			t.Errorf("%d: /demo does not offer to book one: %+v", status, got)
		}
		if status, page := f.browser().get("/demo?team=1"); status != http.StatusOK || shown(t, page).Page != "demo" || !shown(t, page).ForTeam {
			t.Errorf("%d: the team has no form of its own to start a demo from", status)
		}
		open := newFront(t, demoSet, nil)
		if _, page := open.browser().get("/demo"); shown(t, page).ForTeam {
			t.Error("a form any visitor starts a demo from says it is the team's")
		}
	})
}

func TestAVisitorWhoIsNotStaffIsRefusedADemoAndTheStaffAreNot(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, staffSet(), nil)
		status, page, _ := f.browser().post("/demo", url.Values{"name": {"Ada"}, "email": {"ada@acme.com"}, "company": {"Acme"}})
		if got := shown(t, page); status != http.StatusForbidden || got.Page != "demo" || !strings.Contains(got.Error, "Demos are given by the gusto team") || got.Email != "ada@acme.com" || !got.ForTeam {
			t.Errorf("%d: a visitor started a demo: %+v", status, got)
		}
		if orgs, _ := f.store.Orgs(t.Context()); len(orgs) != 0 {
			t.Errorf("a refused demo made an organization: %v", orgs)
		}
		startDemoAs(t, f, "Priya Nair", "priya@latentic.com", "Acme")
	})
}
