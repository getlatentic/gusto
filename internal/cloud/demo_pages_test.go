package cloud

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// demoFront is a gusto cloud server that offers a demo and starts an organization's server, the fake, for each.
func demoFront(t *testing.T, extra ...string) *front {
	t.Helper()
	return newFront(t, demoSet, func(c *Config) {
		c.Controller = controlled(t, c.Store, extra...)
		c.ComputersWait = 50 * time.Millisecond
	})
}

func startDemoAs(t *testing.T, f *front, name, email, company string) *browser {
	t.Helper()
	b := f.browser()
	status, _, header := b.post("/demo", url.Values{"name": {name}, "email": {email}, "company": {company}})
	if status != http.StatusSeeOther || header.Get("Location") != "/demo/wait" {
		t.Fatalf("%d %v", status, header)
	}
	return b
}

func TestAHostThatOffersNoDemoHasNoDemoPage(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		if status, _ := f.browser().get("/demo"); status != http.StatusNotFound {
			t.Errorf("%d", status)
		}
		if status, _, _ := f.browser().post("/demo", url.Values{"name": {"A"}, "email": {"a@acme.com"}, "company": {"Acme"}}); status != http.StatusNotFound {
			t.Errorf("%d", status)
		}
	})
}

func TestAVisitorStartsADemoWaitsForItAndIsSentInOnceWhenItsTimeStarts(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t, "FAKE_DELAY=1500ms")
		if status, page := f.browser().get("/demo"); status != http.StatusOK || shown(t, page).Page != "demo" || shown(t, page).DemoLeft != "30 minutes" || shown(t, page).Book != "https://cal.example/gusto" {
			t.Fatalf("%d %+v", status, shown(t, page))
		}
		b := startDemoAs(t, f, "Ada Lovelace", "ada@acme.com", "Acme Research")
		_, waiting := b.get("/demo/wait")
		if got := shown(t, waiting); got.Page != "demo-wait" || !strings.HasPrefix(got.DemoWords, "Getting your demo ready") || len(got.Steps) == 0 || got.CanOpen || !strings.Contains(waiting, `http-equiv="refresh" content="1"`) {
			t.Errorf("a demo that is not ready says so and looks again: %+v", got)
		}
		org, _, _ := f.store.Org(t.Context(), demoSlugOf(t, f))
		if org.Demo == nil || !org.Demo.Ready.IsZero() {
			t.Errorf("the time started before the server was ready: %+v", org.Demo)
		}
		f.server.Wait()
		status, _, header := b.do("GET", "/demo/wait", nil, nil)
		if status != http.StatusSeeOther || header.Get("Location") != "/open" {
			t.Fatalf("a demo that is ready sends the visitor in: %d %v", status, header)
		}
		if org, _, _ = f.store.Org(t.Context(), org.Slug); org.Demo.Ready.IsZero() || org.Demo.Expires.IsZero() {
			t.Error("the time did not start when the visitor was sent in")
		}
		status, page := b.get("/demo/wait")
		if got := shown(t, page); status != http.StatusOK || !got.CanOpen || got.Org == nil || got.Org.Slug != org.Slug || got.DemoLeft == "" || got.DemoWords != "" || !got.Extendable {
			t.Errorf("%d %+v", status, got)
		}
		status, _, header = b.get2("/open")
		if status != http.StatusSeeOther || !strings.HasPrefix(header.Get("Location"), "http://"+org.Slug+".cloud.test/auth/entry?t=") {
			t.Errorf("the visitor opens their own organization: %d %v", status, header)
		}
	})
}

func demoSlugOf(t *testing.T, f *front) string {
	t.Helper()
	orgs, err := f.store.Orgs(t.Context())
	if err != nil || len(orgs) != 1 {
		t.Fatalf("%v %v", orgs, err)
	}
	return orgs[0].Slug
}

func TestADemosPagesCarryItsCountdownAndWhereToBookAndAnOrdinaryHostsDoNot(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		f.server.Wait()
		b.get("/demo/wait")
		slug := demoSlugOf(t, f)
		_, body, header := b.at(slug+".cloud.test", "GET", "/.gusto/lease", nil)
		if !strings.Contains(header.Get("Content-Type"), "json") || !strings.Contains(body, `"demo":true`) || !strings.Contains(body, `"book":"https://cal.example/gusto"`) ||
			!strings.Contains(body, `"extend":"http://cloud.test/demo/wait"`) || !secondsLeftNearAFullDemo(body) {
			t.Errorf("%s", body)
		}
		if status, _, _ := b.at("cloud.test", "GET", "/.gusto/lease", nil); status != http.StatusNotFound {
			t.Errorf("gusto cloud has no lease of its own here: %d", status)
		}
	})
}

func TestADemoIsExtendedOnceFromThePage(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		f.server.Wait()
		b.get("/demo/wait")
		slug := demoSlugOf(t, f)
		before, _, _ := f.store.DemoLeft(t.Context(), slug)
		if status, _, header := b.post("/demo/extend", nil); status != http.StatusSeeOther || header.Get("Location") != "/demo/wait" {
			t.Fatalf("%d %v", status, header)
		}
		after, _, _ := f.store.DemoLeft(t.Context(), slug)
		if !after.Expires.Equal(before.Expires.Add(10 * time.Minute)) {
			t.Errorf("%v then %v", before.Expires, after.Expires)
		}
		_, _, header := b.post("/demo/extend", nil)
		if !strings.Contains(header.Get("Location"), "notice=") {
			t.Errorf("a second extension is said to be refused: %v", header)
		}
		if _, page := b.get("/demo/wait?notice=This+demo+cannot+be+extended+any+more."); shown(t, page).Extendable || shown(t, page).Notice != "This demo cannot be extended any more." {
			t.Errorf("an extended demo is not offered another: %+v", shown(t, page))
		}
	})
}

func TestWhenADemosTimeIsUpItsServerComputersAndOrganizationAreRemovedAndTheAddressSaysSoAndOffersACall(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		f.server.Wait()
		b.get("/demo/wait")
		slug := demoSlugOf(t, f)
		f.signedUp("cy@initech.com")
		f.server.Wait()
		if address, state := f.server.Controller.Route(t.Context(), slug); state != Ready || address == "" {
			t.Fatalf("%q %q", address, state)
		}
		f.server.EndDueDemos(t.Context())
		f.server.ender.running.Wait()
		if _, found, _ := f.store.Org(t.Context(), slug); !found {
			t.Fatal("a demo with time left was ended")
		}
		f.store.now = func() time.Time { return time.Now().Add(time.Hour) }
		f.server.EndDueDemos(t.Context())
		f.server.ender.running.Wait()

		if _, found, _ := f.store.Org(t.Context(), slug); found {
			t.Error("the organization is still there")
		}
		if address, state := f.server.Controller.Route(t.Context(), slug); address != "" || state != "" {
			t.Errorf("the server was not stopped and forgotten: %q %q", address, state)
		}
		status, body, _ := b.at(slug+".cloud.test", "GET", "/", nil)
		if got := shown(t, body); status != http.StatusGone || got.Page != "demo-end" || !got.Ended || got.Ending || got.Book != "https://cal.example/gusto" || got.Name != "Ada" {
			t.Errorf("%d %+v", status, got)
		}
		// A page still open on the demo asks how long is left, and is told it has ended, so it goes to the page above.
		if status, lease, _ := b.at(slug+".cloud.test", "GET", "/.gusto/lease", nil); status != http.StatusOK || !strings.Contains(lease, `"ended":true`) {
			t.Errorf("an ended demo's lease: %d %s", status, lease)
		}
		styled(t, b, slug+".cloud.test", body)
		if status, _, header := b.get2("/demo/wait"); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
			t.Errorf("the visitor's sign-in went with their account: %d %v", status, header)
		}
		if lead, _ := f.store.LeadOf(t.Context(), slug); lead.Reason != "its time was up" || lead.Ended.IsZero() {
			t.Errorf("%+v", lead)
		}
		if _, found, _ := f.store.Org(t.Context(), "initech-com"); !found {
			t.Error("an organization that is not a demo was ended")
		}
	})
}

func TestADemoThatIsRefusedSaysWhyAndKeepsWhatWasTyped(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		status, page, _ := f.browser().post("/demo", url.Values{"name": {""}, "email": {"ada@acme.com"}, "company": {"Acme"}})
		if got := shown(t, page); status != http.StatusUnprocessableEntity || got.Page != "demo" || !strings.Contains(got.Error, "Say who you are") || got.Email != "ada@acme.com" || got.OrgName != "Acme" {
			t.Errorf("%d %+v", status, got)
		}
		if orgs, _ := f.store.Orgs(t.Context()); len(orgs) != 0 {
			t.Error("a refusal made an organization")
		}
		startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		status, page, _ = f.browser().post("/demo", url.Values{"name": {"Ada"}, "email": {"ada@acme.com"}, "company": {"Acme"}})
		if status != http.StatusConflict || !strings.Contains(shown(t, page).Error, "demo running") {
			t.Errorf("%d %q", status, shown(t, page).Error)
		}
		f.server.Wait()
	})
}

func TestDemosAreLimitedInNumberAndAVisitorOverTheLimitIsToldToBookACall(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		set := demoSet
		set.Demo.MaxOrgs = 1
		f := newFront(t, set, func(c *Config) { c.Controller = controlled(t, c.Store) })
		startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		status, page, _ := f.browser().post("/demo", url.Values{"name": {"Bob"}, "email": {"bob@globex.com"}, "company": {"Globex"}})
		if got := shown(t, page); status != http.StatusServiceUnavailable || !strings.Contains(got.Error, "Every demo is in use") || got.Book != "https://cal.example/gusto" {
			t.Errorf("%d %+v", status, got)
		}
		f.server.Wait()
	})
}

func landingAt(t *testing.T, set Settings) (int, string, http.Header) {
	t.Helper()
	f := newFront(t, set, nil)
	return f.browser().do("GET", "/", nil, nil)
}

func TestWhereADemoIsOfferedTheFrontPageIsTheLandingPageAndSignUpMovesToItsOwnAddress(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		status, page, header := landingAt(t, demoSet)
		if got := shown(t, page); got.Page != "landing" || got.Book != "https://cal.example/gusto" || got.StartingCredit != StartingCredit {
			t.Errorf("the landing page does not offer sign-up and a call: %+v", got)
		}
		if csp := header.Get("Content-Security-Policy"); status != http.StatusOK || strings.Contains(csp, "frame-src") || strings.Contains(csp, "media-src") || strings.Contains(csp, "https:") || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%d: the landing page opens up more than its own: %s", status, csp)
		}
		f := newFront(t, demoSet, nil)
		if status, page := f.browser().get("/signup"); status != http.StatusOK || shown(t, page).Page != "signup" {
			t.Errorf("sign-up is still there: %d", status)
		}
		styled(t, f.browser(), "cloud.test", page)
		if status, page, _ := landingAt(t, Settings{}); status != http.StatusOK || shown(t, page).Page != "signup" {
			t.Errorf("a host that offers no demo still has the sign-up page at the front: %d", status)
		}
	})
}

// The landing page is shown to everybody, and a person signed in is offered gusto from it, which takes them to their demo.
func TestASignedInVisitorSeesTheLandingPageAndOpensTheirDemoFromIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		if status, page := b.get("/"); status != http.StatusOK || shown(t, page).Page != "landing" || !shown(t, page).SignedIn {
			t.Errorf("a signed-in visitor was not shown the landing page with a way into gusto: %d", status)
		}
		if status, _, header := b.get2("/app"); status != http.StatusSeeOther || header.Get("Location") != "/demo/wait" {
			t.Errorf("%d %v", status, header)
		}
		if status, page := f.browser().get("/"); status != http.StatusOK || shown(t, page).Page != "landing" || shown(t, page).SignedIn {
			t.Errorf("a visitor is not offered a sign-in: %d", status)
		}
		if status, _, header := f.browser().get2("/app"); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
			t.Errorf("a visitor opening gusto was sent to %d %v", status, header)
		}
		f.server.Wait()
	})
}

func TestAVisitorWhoEndsTheirDemoHasItDeletedAtOnceAndIsToldWhenItIsDone(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		f.server.Wait()
		b.get("/demo/wait")
		slug := demoSlugOf(t, f)

		if status, page, _ := b.get2("/demo/end"); status != http.StatusOK || shown(t, page).Page != "demo-stop" {
			t.Fatalf("the visitor is not asked: %d %+v", status, shown(t, page))
		}
		if _, found, _ := f.store.Org(t.Context(), slug); !found {
			t.Fatal("asking ended the demo")
		}
		status, _, header := b.post("/demo/end", url.Values{})
		f.server.ender.running.Wait()
		if status != http.StatusSeeOther || header.Get("Location") != "/demo/ended/"+slug {
			t.Fatalf("%d %v", status, header)
		}
		if _, found, _ := f.store.Org(t.Context(), slug); found {
			t.Error("the organization is still there")
		}
		if lead, _ := f.store.LeadOf(t.Context(), slug); lead.Reason != "the visitor ended it" || lead.Ended.IsZero() {
			t.Errorf("%+v", lead)
		}
		status, page, _ := b.get2("/demo/ended/" + slug)
		if got := shown(t, page); status != http.StatusOK || got.Page != "demo-end" || !got.Ended || got.Ending || got.Book != "https://cal.example/gusto" {
			t.Errorf("%d %+v", status, got)
		}
		if status, _, _ := f.browser().get2("/demo/ended/nobody"); status != http.StatusNotFound {
			t.Errorf("a demo that never was has a page: %d", status)
		}
	})
}

func TestAVisitorWhoHasHadTheirDemosForTheDayIsToldToBookACall(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		set := demoSet
		set.Demo.PerPerson = 1
		f := newFront(t, set, func(c *Config) {
			c.Controller = controlled(t, c.Store)
			c.ComputersWait = 50 * time.Millisecond
		})
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		f.server.Wait()
		b.post("/demo/end", url.Values{})
		f.server.ender.running.Wait()
		status, page, _ := f.browser().post("/demo", url.Values{"name": {"Ada"}, "email": {"ada@acme.com"}, "company": {"Acme"}})
		if got := shown(t, page); status != http.StatusTooManyRequests || !strings.Contains(got.Error, "Book a call") || !strings.Contains(got.Error, "demos this address or company gets in a day") {
			t.Errorf("%d %q", status, got.Error)
		}
	})
}

// secondsLeftNearAFullDemo says the lease reports about the thirty minutes a demo is given. It is read a moment after the demo
// started, and on a machine under load that moment is longer than it is on a quiet one.
func secondsLeftNearAFullDemo(body string) bool {
	var lease struct {
		Seconds int `json:"seconds"`
	}
	return json.Unmarshal([]byte(body), &lease) == nil && lease.Seconds > 1740 && lease.Seconds <= 1800
}

// A demo's visitor cannot confirm an address, so its organization page sends them to the demo page and asks for nothing.
func TestADemosOrganizationPageAsksNoOneToConfirmAnAddress(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := startDemoAs(t, demoFront(t), "Ada", "ada@acme.com", "Acme")
		status, page := b.get("/org")
		if got := shown(t, page); status != http.StatusOK || got.Page != "org" || got.Org == nil || !got.Org.Demo {
			t.Errorf("%d %+v", status, got)
		}
	})
}
