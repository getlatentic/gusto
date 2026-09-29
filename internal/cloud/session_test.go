package cloud

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cookieFor(t *testing.T, s *Sessions, email string) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	s.Issue(recorder, email)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("%d cookies", len(cookies))
	}
	return cookies[0]
}

func whoAmI(s *Sessions, cookie *http.Cookie) (string, bool) {
	r := httptest.NewRequest("GET", "/", nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return s.Who(r)
}

func TestASessionNamesThePersonUntilItRunsOutAndSurvivesARestart(t *testing.T) {
	root := t.TempDir()
	s, err := OpenSessions(root, false)
	if err != nil {
		t.Fatal(err)
	}
	cookie := cookieFor(t, s, "ada@acme.com")
	if email, ok := whoAmI(s, cookie); !ok || email != "ada@acme.com" {
		t.Fatalf("%q %v", email, ok)
	}
	if info, err := os.Stat(filepath.Join(root, "secret")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the key is private to the server: %v %v", info, err)
	}
	restarted, _ := OpenSessions(root, false)
	if email, ok := whoAmI(restarted, cookie); !ok || email != "ada@acme.com" {
		t.Errorf("a restart signs nobody out: %q %v", email, ok)
	}
	restarted.now = func() time.Time { return time.Now().Add(SessionLife + time.Hour) }
	if _, ok := whoAmI(restarted, cookie); ok {
		t.Error("a session ends")
	}
}

func TestACookieThatWasChangedOrSignedWithAnotherKeyIsNobody(t *testing.T) {
	s, _ := OpenSessions(t.TempDir(), false)
	cookie := cookieFor(t, s, "ada@acme.com")
	other, _ := OpenSessions(t.TempDir(), false)
	if _, ok := whoAmI(other, cookie); ok {
		t.Error("another key accepted it")
	}
	swapped := cookieFor(t, s, "bob@acme.com")
	payload, _, _ := splitCookie(cookie.Value)
	_, signature, _ := splitCookie(swapped.Value)
	forged := *cookie
	forged.Value = payload + "." + signature
	if _, ok := whoAmI(s, &forged); ok {
		t.Error("a payload with another person's signature was accepted")
	}
	for _, bad := range []string{"", ".", "a.b", "nodot", cookie.Value + "x"} {
		if _, ok := whoAmI(s, &http.Cookie{Name: cookie.Name, Value: bad}); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
	if _, ok := whoAmI(s, nil); ok {
		t.Error("no cookie is nobody")
	}
}

func TestOverHTTPSTheCookieIsLockedToTheHostAndSecure(t *testing.T) {
	s, _ := OpenSessions(t.TempDir(), true)
	cookie := cookieFor(t, s, "ada@acme.com")
	if cookie.Name != "__Host-haas_cloud" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Errorf("%+v", cookie)
	}
}

func splitCookie(value string) (payload, signature string, ok bool) {
	for i := len(value) - 1; i >= 0; i-- {
		if value[i] == '.' {
			return value[:i], value[i+1:], true
		}
	}
	return value, "", false
}
