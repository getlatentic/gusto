package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/entry"
)

var entryKey = []byte("0123456789abcdef0123456789abcdef")

// behindAFrontDoor is a server that is the organization acme-com's in a hosted gusto.
func behindAFrontDoor(t *testing.T) *site {
	t.Helper()
	s := open(t)
	s.server.UseFrontDoor(FrontDoor{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
	s.handler = s.server.Handler()
	return s
}

func (s *site) enterWith(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()
	return s.ask(t, "GET", "/auth/entry?t="+url.QueryEscape(token), nil)
}

func tokenFor(t *testing.T, org, email, role string, at time.Time) string {
	t.Helper()
	token, err := entry.Mint(entryKey, org, email, role, at)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAPersonWhoIsSentByTheFrontDoorIsSignedInAsWhoTheyAreWithNoPassword(t *testing.T) {
	s := behindAFrontDoor(t)
	answer := s.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now()))
	if answer.Code != http.StatusSeeOther || answer.Header().Get("Location") != "/" {
		t.Fatalf("%d %v", answer.Code, answer.Header())
	}
	cookie := cookieOf(t, answer)
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("%+v", cookie)
	}
	if left := time.Until(cookie.Expires); left > sessionLife+time.Minute || left < sessionLife-time.Minute {
		t.Errorf("a front door's word is trusted for an hour, not %s", left)
	}
	s.cookie = cookie
	if home := s.ask(t, "GET", "/chat", nil); home.Code != http.StatusOK {
		t.Errorf("the person is in: %d", home.Code)
	}
	probe := httptest.NewRequest("GET", "/", nil)
	probe.AddCookie(cookie)
	if who := s.server.keeper.who(probe); who != "ada@acme.com" {
		t.Errorf("the job they start is theirs: %q", who)
	}
}

func TestARunnerIsNotAnAdminAndAnAdminIs(t *testing.T) {
	// The record of changes is an admin's page, so each server keeps one.
	admin, _ := recording(t)
	runner, _ := recording(t)
	for _, s := range []*site{admin, runner} {
		s.server.UseFrontDoor(FrontDoor{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
		s.handler = s.server.Handler()
	}
	admin.cookie = cookieOf(t, admin.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	runner.cookie = cookieOf(t, runner.enterWith(t, tokenFor(t, "acme-com", "bob@acme.com", "runner", time.Now())))
	if got := admin.ask(t, "GET", "/admin/records", nil).Code; got != http.StatusOK {
		t.Errorf("an admin reads the record of changes: %d", got)
	}
	if got := runner.ask(t, "GET", "/admin/records", nil).Code; got == http.StatusOK {
		t.Errorf("a runner does not: %d", got)
	}
}

func TestATokenThatIsUsedOldForAnotherOrganizationOrMadeWithAnotherKeyIsRefusedWithAPage(t *testing.T) {
	s := behindAFrontDoor(t)
	good := tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())
	if s.enterWith(t, good).Code != http.StatusSeeOther {
		t.Fatal("the first use works")
	}
	other, _ := entry.Mint([]byte("another key another key another!"), "acme-com", "ada@acme.com", "admin", time.Now())
	for name, token := range map[string]string{
		"used already":    good,
		"old":             tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now().Add(-2*entry.Life)),
		"another org's":   tokenFor(t, "globex-com", "ada@acme.com", "admin", time.Now()),
		"another key's":   other,
		"not a token":     "nonsense",
		"no token at all": "",
	} {
		answer := s.enterWith(t, token)
		if answer.Code != http.StatusForbidden || answer.Header().Get("Location") != "" || !strings.Contains(answer.Body.String(), "https://cloud.test/org?org=acme-com") {
			t.Errorf("%s: %d %v\n%s", name, answer.Code, answer.Header(), answer.Body)
		}
		for _, each := range answer.Result().Cookies() {
			if each.Name == cookieName && each.Value != "" {
				t.Errorf("%s: a cookie was given", name)
			}
		}
	}
}

func TestBehindAFrontDoorThereIsNoPasswordOrProviderToSignInWith(t *testing.T) {
	s := behindAFrontDoor(t)
	if login := s.ask(t, "GET", "/login", nil); login.Code != http.StatusSeeOther || login.Header().Get("Location") != "https://cloud.test/open?org=acme-com" {
		t.Errorf("a stranger is sent to the front door: %d %v", login.Code, login.Header())
	}
	if got := s.ask(t, "POST", "/login", url.Values{"password": {password}}); got.Code == http.StatusSeeOther && got.Header().Get("Location") == "/" {
		t.Error("the shared password still signs somebody in")
	}
	if got := s.ask(t, "GET", "/auth/start", nil); got.Code != http.StatusNotFound {
		t.Errorf("a provider's sign-in is not here: %d", got.Code)
	}
	if home := s.ask(t, "GET", "/", nil); home.Code != http.StatusSeeOther || home.Header().Get("Location") != "/login" {
		t.Errorf("%d %v", home.Code, home.Header())
	}
}

// Signing out here goes on to the front door with a token that asks it to sign the person out too, or the front door
// would let them straight back in. The policy lets the form's redirect go there.
func TestSigningOutGoesOnToSignOutOfTheFrontDoor(t *testing.T) {
	s := behindAFrontDoor(t)
	s.cookie = cookieOf(t, s.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	out := s.ask(t, "POST", "/logout", url.Values{})
	to, err := url.Parse(out.Header().Get("Location"))
	if out.Code != http.StatusSeeOther || err != nil || to.Scheme+"://"+to.Host+to.Path != "https://cloud.test/signout" || to.Query().Get("org") != "acme-com" {
		t.Fatalf("%d %v", out.Code, out.Header())
	}
	if claims, err := entry.VerifyExit(entryKey, "acme-com", to.Query().Get("t"), time.Now()); err != nil || claims.Email != "ada@acme.com" {
		t.Errorf("the front door is not asked to sign ada out: %+v %v", claims, err)
	}
	if policy := out.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "form-action 'self' https://cloud.test;") {
		t.Errorf("the browser would refuse the way on to the front door: %s", policy)
	}
	if home := s.ask(t, "GET", "/chat", nil); home.Code != http.StatusSeeOther {
		t.Errorf("ada is still signed in here: %d", home.Code)
	}
}

func TestASignInByTheFrontDoorIsInTheRecordOfChanges(t *testing.T) {
	s, log := recording(t)
	s.server.UseFrontDoor(FrontDoor{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
	s.handler = s.server.Handler()
	s.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now()))
	signins := ofKind(written(t, log), changelog.SignedIn)
	if len(signins) != 1 || signins[0].Who != "ada@acme.com" || signins[0].What["how"] != "gusto cloud" {
		t.Errorf("%+v", signins)
	}
}

func cookieOf(t *testing.T, answer *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, each := range answer.Result().Cookies() {
		if each.Name == cookieName && each.Value != "" {
			return each
		}
	}
	return nil
}

// A person may be in several organizations, and the menu leads back to the front door's choice of them.
func TestBehindAFrontDoorTheMenuLeadsToThePersonsOrganizations(t *testing.T) {
	behind := behindAFrontDoor(t)
	behind.cookie = cookieOf(t, behind.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	if page := behind.ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, `href="https://cloud.test/orgs"`) {
		t.Errorf("the menu does not lead to the person's organizations:\n%s", page[:min(len(page), 900)])
	}
}

// A host that removes itself warns on every page, and the warning links to where it is extended, which
// is the front door. A server that is not behind one has nowhere to send a person, and carries no script.
func TestAnOrganizationsPagesCarryTheLeaseScriptOnlyBehindAFrontDoor(t *testing.T) {
	behind := behindAFrontDoor(t)
	behind.cookie = cookieOf(t, behind.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	page := behind.ask(t, "GET", "/chat", nil).Body.String()
	if !strings.Contains(page, `src="/static/lease.js`) || !strings.Contains(page, `data-extend="https://cloud.test/lease?org=acme-com"`) {
		t.Errorf("the page does not warn of the end of the host:\n%s", page[:min(len(page), 900)])
	}
	if script := behind.ask(t, "GET", "/static/lease.js", nil); script.Code != http.StatusOK || !strings.Contains(script.Body.String(), "/.gusto/lease") {
		t.Errorf("the script is not served: %d", script.Code)
	}

	if !strings.Contains(page, `src="/static/guide.js`) {
		t.Error("a demo host's pages do not carry the guided tour")
	}
	if script := behind.ask(t, "GET", "/static/guide.js", nil); script.Code != http.StatusOK || !strings.Contains(script.Body.String(), "gusto:demo") {
		t.Errorf("the tour is not served, or does not wait for the demo signal: %d", script.Code)
	}

	alone := open(t)
	alone.signIn(t)
	if page := alone.ask(t, "GET", "/", nil).Body.String(); strings.Contains(page, "lease.js") || strings.Contains(page, "guide.js") {
		t.Error("a server with no front door carries the script")
	}
}

// The mark at the top of the rail opens the organization switcher, which asks the front door for the person's
// organizations, and the side panel asks it whether they have confirmed their address: the page's policy lets it ask
// there and nowhere else. A server on its own has neither.
func TestTheRailsMarkSwitchesBetweenThePersonsOrganizationsBehindAFrontDoor(t *testing.T) {
	s := open(t)
	s.server.UseFrontDoor(FrontDoor{Key: entryKey, Org: "acme-com", Name: "Acme Research", URL: "https://cloud.test"})
	s.handler = s.server.Handler()
	s.cookie = cookieOf(t, s.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "runner", time.Now())))
	page := s.ask(t, "GET", "/chat", nil)
	body := page.Body.String()
	for _, want := range []string{`data-orgs aria-label="Organizations"`, `data-from="https://cloud.test/orgs.json"`, `data-current="acme-com"`, `font-semibold text-ink">Acme Research</span>`, `href="https://cloud.test/orgs"`, "/static/orgs.js",
		`data-reminder data-from="https://cloud.test/account.json" data-send="https://cloud.test/account/confirm"`, "/static/reminder.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page has %s:\n%s", want, body)
		}
	}
	if policy := page.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "connect-src 'self' https://cloud.test;") {
		t.Errorf("the switcher may ask the front door: %s", policy)
	}

	alone := open(t)
	alone.signIn(t)
	page = alone.ask(t, "GET", "/chat", nil)
	if body := page.Body.String(); strings.Contains(body, "data-orgs") || strings.Contains(body, "data-reminder") || !strings.Contains(body, `class="brand-mark mb-2 grid size-10 place-items-center" href="/chat"`) {
		t.Errorf("a server on its own has its mark link home:\n%s", body)
	}
	if policy := page.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "connect-src 'self';") || !strings.Contains(policy, "form-action 'self';") {
		t.Errorf("a server on its own asks nowhere else: %s", policy)
	}
}
