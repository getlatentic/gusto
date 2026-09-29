package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/entry"
)

var entryKey = []byte("0123456789abcdef0123456789abcdef")

// behindAFrontDoor is a server that is the organisation acme-com's in a hosted haas.
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
	if home := s.ask(t, "GET", "/", nil); home.Code != http.StatusOK {
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
	if got := admin.ask(t, "GET", "/changes", nil).Code; got != http.StatusOK {
		t.Errorf("an admin reads the record of changes: %d", got)
	}
	if got := runner.ask(t, "GET", "/changes", nil).Code; got == http.StatusOK {
		t.Errorf("a runner does not: %d", got)
	}
}

func TestATokenThatIsUsedOldForAnotherOrganisationOrMadeWithAnotherKeyIsRefusedWithAPage(t *testing.T) {
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
		if answer.Code != http.StatusForbidden || answer.Header().Get("Location") != "" || !strings.Contains(answer.Body.String(), "https://cloud.test/org") {
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
	if login := s.ask(t, "GET", "/login", nil); login.Code != http.StatusSeeOther || login.Header().Get("Location") != "https://cloud.test/open" {
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

func TestSigningOutGoesBackToTheFrontDoor(t *testing.T) {
	s := behindAFrontDoor(t)
	s.cookie = cookieOf(t, s.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	out := s.ask(t, "POST", "/logout", url.Values{})
	if out.Code != http.StatusSeeOther || out.Header().Get("Location") != "https://cloud.test/org" {
		t.Errorf("%d %v", out.Code, out.Header())
	}
}

func TestASignInByTheFrontDoorIsInTheRecordOfChanges(t *testing.T) {
	s, log := recording(t)
	s.server.UseFrontDoor(FrontDoor{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
	s.handler = s.server.Handler()
	s.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now()))
	signins := ofKind(written(t, log), changelog.SignedIn)
	if len(signins) != 1 || signins[0].Who != "ada@acme.com" || signins[0].What["how"] != "the front door" {
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
