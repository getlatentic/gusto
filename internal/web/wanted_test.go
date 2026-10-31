package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// signInFrom signs in with what a browser carried, and says where it was sent.
func signInFrom(t *testing.T, site *site, carried []*http.Cookie) string {
	t.Helper()
	request := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"password": {password}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range carried {
		request.AddCookie(cookie)
	}
	answer := httptest.NewRecorder()
	site.handler.ServeHTTP(answer, request)
	return answer.Header().Get("Location")
}

// A page asked for before signing in is the page signing in goes to, once; a path that would leave the server is never kept.
func TestSigningInGoesToThePageThatWasAskedFor(t *testing.T) {
	site := open(t)
	asked := site.ask(t, "GET", "/admin/usage?month=2026-10", nil)
	if asked.Header().Get("Location") != "/login" {
		t.Fatalf("a stranger went to %q", asked.Header().Get("Location"))
	}
	if where := signInFrom(t, site, asked.Result().Cookies()); where != "/admin/usage?month=2026-10" {
		t.Errorf("signing in went to %q", where)
	}
	if where := signInFrom(t, site, nil); where != "/" {
		t.Errorf("with nothing asked for, signing in went to %q", where)
	}
	for _, away := range []string{"//evil.example/x", "/\\evil.example", "/auth/entry?t=x"} {
		if ownPath(away) {
			t.Errorf("%q would be kept", away)
		}
	}
	if where := signInFrom(t, site, []*http.Cookie{{Name: wantedCookie, Value: "//evil.example/x"}}); where != "/" {
		t.Errorf("a cookie naming another site sent the person to %q", where)
	}
}
