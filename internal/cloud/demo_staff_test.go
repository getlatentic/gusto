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
		for _, want := range []string{`href="/signup"`, "Start free", "Book a demo", `href="https://cal.example/gusto"`, "Free to start, with $5 of gusto's models. Then add your own model key."} {
			if !strings.Contains(page, want) {
				t.Errorf("the landing page does not say %q", want)
			}
		}
		if status != http.StatusOK || strings.Contains(page, `href="/demo"`) || strings.Contains(page, "Try it now") || strings.Contains(page, "isolated demo") {
			t.Errorf("%d: the landing page still offers visitors a demo", status)
		}
		if _, page, _ := landingAt(t, demoSet); !strings.Contains(page, `href="/demo"`) || strings.Contains(page, "Start free") {
			t.Error("a host whose demos are open to anyone no longer offers them")
		}

		f := newFront(t, staffSet(), nil)
		status, page = f.browser().get("/demo")
		if status != http.StatusOK || !strings.Contains(page, "Book a demo") || !strings.Contains(page, `href="https://cal.example/gusto"`) || !strings.Contains(page, `href="/signup"`) || strings.Contains(page, `action="/demo"`) {
			t.Errorf("%d: /demo does not offer to book one\n%s", status, page)
		}
		if status, page := f.browser().get("/demo?team=1"); status != http.StatusOK || !strings.Contains(page, `action="/demo"`) {
			t.Errorf("%d: the team has no form to start a demo from", status)
		}
	})
}

func TestAVisitorWhoIsNotStaffIsRefusedADemoAndTheStaffAreNot(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, staffSet(), nil)
		status, page, _ := f.browser().post("/demo", url.Values{"name": {"Ada"}, "email": {"ada@acme.com"}, "company": {"Acme"}})
		if status != http.StatusForbidden || !strings.Contains(page, "Demos are given by the gusto team") || !strings.Contains(page, `value="ada@acme.com"`) {
			t.Errorf("%d: a visitor started a demo\n%s", status, page)
		}
		if orgs, _ := f.store.Orgs(); len(orgs) != 0 {
			t.Errorf("a refused demo made an organization: %v", orgs)
		}
		startDemoAs(t, f, "Priya Nair", "priya@latentic.com", "Acme")
	})
}
