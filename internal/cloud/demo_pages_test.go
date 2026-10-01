package cloud

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// demoFront is a front door that offers a demo and starts an organisation's server, the fake, for each.
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
	f := newFront(t, Settings{}, nil)
	if status, _ := f.browser().get("/demo"); status != http.StatusNotFound {
		t.Errorf("%d", status)
	}
	if status, _, _ := f.browser().post("/demo", url.Values{"name": {"A"}, "email": {"a@acme.com"}, "company": {"Acme"}}); status != http.StatusNotFound {
		t.Errorf("%d", status)
	}
}

func TestAVisitorStartsADemoWaitsForItAndIsSentInOnceWhenItsTimeStarts(t *testing.T) {
	f := demoFront(t, "FAKE_DELAY=1500ms")
	if status, page := f.browser().get("/demo"); status != http.StatusOK || !strings.Contains(page, "Try haas for 30 minutes") || !strings.Contains(page, "https://cal.example/haas") {
		t.Fatalf("%d\n%s", status, page)
	}
	b := startDemoAs(t, f, "Ada Lovelace", "ada@acme.com", "Acme Research")
	_, waiting := b.get("/demo/wait")
	if !strings.Contains(waiting, "Getting your demo ready") || !strings.Contains(waiting, `http-equiv="refresh"`) || strings.Contains(waiting, `href="/open"`) {
		t.Errorf("a demo that is not ready says so and looks again:\n%s", waiting)
	}
	org, _, _ := f.store.Org(demoSlugOf(t, f))
	if org.Demo == nil || !org.Demo.Ready.IsZero() {
		t.Errorf("the time started before the server was ready: %+v", org.Demo)
	}
	f.server.Wait()
	status, _, header := b.do("GET", "/demo/wait", nil, nil)
	if status != http.StatusSeeOther || header.Get("Location") != "/open" {
		t.Fatalf("a demo that is ready sends the visitor in: %d %v", status, header)
	}
	if org, _, _ = f.store.Org(org.Slug); org.Demo.Ready.IsZero() || org.Demo.Expires.IsZero() {
		t.Error("the time did not start when the visitor was sent in")
	}
	status, page := b.get("/demo/wait")
	if status != http.StatusOK || !strings.Contains(page, `href="/open"`) || !strings.Contains(page, "left") || !strings.Contains(page, "10 more minutes") {
		t.Errorf("%d\n%s", status, page)
	}
	status, _, header = b.get2("/open")
	if status != http.StatusSeeOther || !strings.HasPrefix(header.Get("Location"), "http://"+org.Slug+".cloud.test/auth/entry?t=") {
		t.Errorf("the visitor opens their own organisation: %d %v", status, header)
	}
}

func demoSlugOf(t *testing.T, f *front) string {
	t.Helper()
	orgs, err := f.store.Orgs()
	if err != nil || len(orgs) != 1 {
		t.Fatalf("%v %v", orgs, err)
	}
	return orgs[0].Slug
}

func TestADemosPagesCarryItsCountdownAndWhereToBookAndAnOrdinaryHostsDoNot(t *testing.T) {
	f := demoFront(t)
	b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	f.server.Wait()
	b.get("/demo/wait")
	slug := demoSlugOf(t, f)
	_, body, header := b.at(slug+".cloud.test", "GET", "/.haas/lease", nil)
	if !strings.Contains(header.Get("Content-Type"), "json") || !strings.Contains(body, `"demo":true`) || !strings.Contains(body, `"book":"https://cal.example/haas"`) ||
		!strings.Contains(body, `"extend":"http://cloud.test/demo/wait"`) || !strings.Contains(body, `"seconds":1799`) && !strings.Contains(body, `"seconds":1800`) {
		t.Errorf("%s", body)
	}
	if status, _, _ := b.at("cloud.test", "GET", "/.haas/lease", nil); status != http.StatusNotFound {
		t.Errorf("the front door has no lease of its own here: %d", status)
	}
}

func TestADemoIsExtendedOnceFromThePage(t *testing.T) {
	f := demoFront(t)
	b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	f.server.Wait()
	b.get("/demo/wait")
	slug := demoSlugOf(t, f)
	before, _, _ := f.store.DemoLeft(slug)
	if status, _, header := b.post("/demo/extend", nil); status != http.StatusSeeOther || header.Get("Location") != "/demo/wait" {
		t.Fatalf("%d %v", status, header)
	}
	after, _, _ := f.store.DemoLeft(slug)
	if !after.Expires.Equal(before.Expires.Add(10 * time.Minute)) {
		t.Errorf("%v then %v", before.Expires, after.Expires)
	}
	_, _, header := b.post("/demo/extend", nil)
	if !strings.Contains(header.Get("Location"), "notice=") {
		t.Errorf("a second extension is said to be refused: %v", header)
	}
	if _, page := b.get("/demo/wait?notice=This+demo+cannot+be+extended+any+more."); strings.Contains(page, "10 more minutes") || !strings.Contains(page, "cannot be extended") {
		t.Errorf("an extended demo is not offered another:\n%s", page)
	}
}

func TestWhenADemosTimeIsUpItsServerComputersAndOrganisationAreRemovedAndTheAddressSaysSoAndOffersACall(t *testing.T) {
	f := demoFront(t)
	b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	f.server.Wait()
	b.get("/demo/wait")
	slug := demoSlugOf(t, f)
	f.signedUp("cy@initech.com")
	f.server.Wait()
	if address, state := f.server.Controller.Route(slug); state != Ready || address == "" {
		t.Fatalf("%q %q", address, state)
	}
	f.server.EndDueDemos()
	f.server.ender.running.Wait()
	if _, found, _ := f.store.Org(slug); !found {
		t.Fatal("a demo with time left was ended")
	}
	f.store.now = func() time.Time { return time.Now().Add(time.Hour) }
	f.server.EndDueDemos()
	f.server.ender.running.Wait()

	if _, found, _ := f.store.Org(slug); found {
		t.Error("the organisation is still there")
	}
	if address, state := f.server.Controller.Route(slug); address != "" || state != "" {
		t.Errorf("the server was not stopped and forgotten: %q %q", address, state)
	}
	status, body, _ := b.at(slug+".cloud.test", "GET", "/", nil)
	if status != http.StatusGone || !strings.Contains(body, "Your demo has ended") || !strings.Contains(body, `href="https://cal.example/haas"`) || !strings.Contains(body, "Ada") {
		t.Errorf("%d\n%s", status, body)
	}
	if status, sheet, header := b.at(slug+".cloud.test", "GET", "/static/cloud.css", nil); status != http.StatusOK || !strings.Contains(header.Get("Content-Type"), "css") || !strings.Contains(sheet, ".brand") {
		t.Errorf("the page of an ended demo has no styles: %d %v", status, header)
	}
	if status, _, header := b.get2("/demo/wait"); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
		t.Errorf("the visitor's sign-in went with their account: %d %v", status, header)
	}
	if lead, _ := f.store.LeadOf(slug); lead.Reason != "its time was up" || lead.Ended.IsZero() {
		t.Errorf("%+v", lead)
	}
	if _, found, _ := f.store.Org("initech-com"); !found {
		t.Error("an organisation that is not a demo was ended")
	}
}

func TestADemoThatIsRefusedSaysWhyAndKeepsWhatWasTyped(t *testing.T) {
	f := demoFront(t)
	status, page, _ := f.browser().post("/demo", url.Values{"name": {""}, "email": {"ada@acme.com"}, "company": {"Acme"}})
	if status != http.StatusUnprocessableEntity || !strings.Contains(page, "Say who you are") || !strings.Contains(page, `value="ada@acme.com"`) || !strings.Contains(page, `value="Acme"`) {
		t.Errorf("%d\n%s", status, page)
	}
	if orgs, _ := f.store.Orgs(); len(orgs) != 0 {
		t.Error("a refusal made an organisation")
	}
	startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	status, page, _ = f.browser().post("/demo", url.Values{"name": {"Ada"}, "email": {"ada@acme.com"}, "company": {"Acme"}})
	if status != http.StatusConflict || !strings.Contains(page, "demo running") {
		t.Errorf("%d\n%s", status, page)
	}
	f.server.Wait()
}

func TestDemosAreLimitedInNumberAndAVisitorOverTheLimitIsToldToBookACall(t *testing.T) {
	set := demoSet
	set.Demo.MaxOrgs = 1
	f := newFront(t, set, func(c *Config) { c.Controller = controlled(t, c.Store) })
	startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	status, page, _ := f.browser().post("/demo", url.Values{"name": {"Bob"}, "email": {"bob@globex.com"}, "company": {"Globex"}})
	if status != http.StatusServiceUnavailable || !strings.Contains(page, "Every demo is in use") || !strings.Contains(page, "cal.example/haas") {
		t.Errorf("%d\n%s", status, page)
	}
	f.server.Wait()
}

func landingAt(t *testing.T, set Settings) (int, string, http.Header) {
	t.Helper()
	f := newFront(t, set, nil)
	return f.browser().do("GET", "/", nil, nil)
}

func TestWhereADemoIsOfferedTheFrontPageIsTheLandingPageAndSignUpMovesToItsOwnAddress(t *testing.T) {
	status, page, header := landingAt(t, demoSet)
	for _, want := range []string{"Beyond AI chat.", "Latentic Agent Cloud", `href="/demo"`, "Start a 30-minute demo", `href="https://cal.example/haas"`, "One place for your organisation", "From agent request to controlled execution", "Bring your agents. Bring your models.", `rel="stylesheet" href="/static/landing.css`} {
		if !strings.Contains(strings.ToLower(page), strings.ToLower(want)) {
			t.Errorf("the landing page does not say %q", want)
		}
	}
	if status != http.StatusOK || !strings.Contains(page, `<video controls preload="metadata" src="/static/demo.mp4" poster="/static/demo-poster.jpg">`) || strings.Contains(page, "<iframe") {
		t.Errorf("%d: the video that comes with haas is shown\n%s", status, page)
	}
	if csp := header.Get("Content-Security-Policy"); strings.Contains(csp, "frame-src") || strings.Contains(csp, "media-src") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("a video of its own opens nothing up: %s", csp)
	}
	f := newFront(t, demoSet, nil)
	if status, page := f.browser().get("/signup"); status != http.StatusOK || !strings.Contains(page, "Create your organisation") {
		t.Errorf("sign-up is still there: %d", status)
	}
	if status, css, _ := f.browser().do("GET", "/static/landing.css", nil, nil); status != http.StatusOK || !strings.Contains(css, ".hero") {
		t.Errorf("its stylesheet is served: %d", status)
	}
	if status, page, _ := landingAt(t, Settings{}); status != http.StatusOK || !strings.Contains(page, "Create your organisation") {
		t.Errorf("a host that offers no demo still has the sign-up page at the front: %d", status)
	}
}

func TestTheLandingPageShowsAVideoFileOrAnEmbeddedPageAndOpensUpOnlyWhatItNeedsAndNeverAnAddressThatIsNotHttps(t *testing.T) {
	for name, tc := range map[string]struct {
		video     string
		tag, csp  string
		notInPage string
	}{
		"a file":          {"https://media.example/haas.mp4", "<video", "media-src 'self' https:", "<iframe"},
		"a page to embed": {"https://www.loom.com/embed/abc123", "<iframe", "frame-src https://www.loom.com", "<video"},
	} {
		set := demoSet
		set.Demo.VideoURL = tc.video
		_, page, header := landingAt(t, set)
		if !strings.Contains(page, tc.tag) || !strings.Contains(page, tc.video) || strings.Contains(page, tc.notInPage) || !strings.Contains(header.Get("Content-Security-Policy"), tc.csp) {
			t.Errorf("%s:\n%s\n%s", name, header.Get("Content-Security-Policy"), page)
		}
	}
	for _, bad := range []string{"http://media.example/haas.mp4", "javascript:alert(1)", "//media.example/x.mp4", "https://user:pw@media.example/x.mp4", "not a url"} {
		set := demoSet
		set.Demo.VideoURL = bad
		_, page, header := landingAt(t, set)
		if strings.Contains(page, "<video") || strings.Contains(page, "<iframe") || strings.Contains(header.Get("Content-Security-Policy"), "frame-src") {
			t.Errorf("%q was shown", bad)
		}
	}
}

func TestASignedInVisitorIsSentToTheirOrganisationAndNotShownTheLandingPage(t *testing.T) {
	f := demoFront(t)
	b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	if status, _, header := b.get2("/"); status != http.StatusSeeOther || header.Get("Location") != "/org" {
		t.Errorf("%d %v", status, header)
	}
	f.server.Wait()
}

func TestTheVideoThatComesWithHaasIsServedAsAVideoThatCanBeSeekedIn(t *testing.T) {
	f := newFront(t, demoSet, nil)
	status, body, header := f.browser().do("GET", "/static/demo.mp4", nil, map[string]string{"Range": "bytes=0-99"})
	if status != http.StatusPartialContent || len(body) != 100 || !strings.HasPrefix(header.Get("Content-Type"), "video/mp4") || !strings.Contains(header.Get("Content-Range"), "bytes 0-99/") {
		t.Errorf("%d %d bytes %q %q", status, len(body), header.Get("Content-Type"), header.Get("Content-Range"))
	}
	if status, _, header := f.browser().do("GET", "/static/demo-poster.jpg", nil, nil); status != http.StatusOK || header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("%d %q", status, header.Get("Content-Type"))
	}
}

func TestAVisitorWhoEndsTheirDemoHasItDeletedAtOnceAndIsToldWhenItIsDone(t *testing.T) {
	f := demoFront(t)
	b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	f.server.Wait()
	b.get("/demo/wait")
	slug := demoSlugOf(t, f)

	if status, page, _ := b.get2("/demo/end"); status != http.StatusOK || !strings.Contains(page, "End your demo now?") {
		t.Fatalf("the visitor is not asked: %d\n%s", status, page)
	}
	if _, found, _ := f.store.Org(slug); !found {
		t.Fatal("asking ended the demo")
	}
	status, _, header := b.post("/demo/end", url.Values{})
	f.server.ender.running.Wait()
	if status != http.StatusSeeOther || header.Get("Location") != "/demo/ended/"+slug {
		t.Fatalf("%d %v", status, header)
	}
	if _, found, _ := f.store.Org(slug); found {
		t.Error("the organisation is still there")
	}
	if lead, _ := f.store.LeadOf(slug); lead.Reason != "the visitor ended it" || lead.Ended.IsZero() {
		t.Errorf("%+v", lead)
	}
	status, page, _ := b.get2("/demo/ended/" + slug)
	if status != http.StatusOK || !strings.Contains(page, "Your demo has ended") || !strings.Contains(page, `href="https://cal.example/haas"`) {
		t.Errorf("%d\n%s", status, page)
	}
	if status, _, _ := f.browser().get2("/demo/ended/nobody"); status != http.StatusNotFound {
		t.Errorf("a demo that never was has a page: %d", status)
	}
}
