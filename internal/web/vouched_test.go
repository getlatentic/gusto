package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/entry"
	"github.com/getlatentic/gusto/internal/roster"
)

// atAPath is acme-com's server behind gusto cloud at a path of gusto cloud's own name, over TLS, with ada its admin and bob a
// runner.
func atAPath(t *testing.T) *site {
	t.Helper()
	here := open(t)
	members := filepath.Join(t.TempDir(), "members.json")
	list := roster.Roster{
		Members:    []roster.Member{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"}},
		Workspaces: []roster.Workspace{{ID: "main", Name: "Acme", Members: []string{"ada@acme.com", "bob@acme.com"}}},
	}
	if err := roster.Write(members, list); err != nil {
		t.Fatal(err)
	}
	here.server.UseRoster(members)
	here.server.UseCloud(Cloud{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
	if err := here.server.UseAddress("https://cloud.test/workspace/acme-com"); err != nil {
		t.Fatal(err)
	}
	here.handler = here.server.Handler()
	return here
}

// vouchedRequest is a request as gusto cloud passes one on, over TLS, saying whose it is with a token.
func vouchedRequest(method, path, token string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	if token != "" {
		request.Header.Set(PersonHeader, token)
	}
	return request
}

func vouchFor(t *testing.T, org, email string, at time.Time) string {
	t.Helper()
	token, err := entry.Vouch(entryKey, org, email, at)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (s *site) serve(request *http.Request) *httptest.ResponseRecorder {
	answer := httptest.NewRecorder()
	s.handler.ServeHTTP(answer, request)
	return answer
}

// A request gusto cloud says is a member's is theirs, with the role the roster gives them. Nobody else's is: not one gusto
// cloud said nothing about, nor one it said about somebody the roster does not have, nor a token that is old, for another
// organization or not gusto cloud's.
func TestAtAPathARequestIsWhoeverGustoCloudSaysItIs(t *testing.T) {
	here := atAPath(t)
	now := time.Now()
	for who, wantAdmin := range map[string]bool{"ada@acme.com": true, "bob@acme.com": false} {
		shell := here.serve(vouchedRequest("GET", "/app/api/shell", vouchFor(t, "acme-com", who, now)))
		if shell.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", who, shell.Code, shell.Body)
		}
		admin := here.serve(vouchedRequest("GET", "/admin", vouchFor(t, "acme-com", who, now)))
		if (admin.Code == http.StatusOK) != wantAdmin {
			t.Errorf("%s reaches the admin console: %d", who, admin.Code)
		}
	}
	enter, _ := entry.Mint(entryKey, "acme-com", "ada@acme.com", "admin", now)
	for name, token := range map[string]string{
		"nobody":                            "",
		"somebody the roster does not have": vouchFor(t, "acme-com", "eve@evil.test", now),
		"an old token":                      vouchFor(t, "acme-com", "ada@acme.com", now.Add(-entry.Life)),
		"another organization's":            vouchFor(t, "globex-com", "ada@acme.com", now),
		"a token to enter with":             enter,
		"a token that is not one":           "ada@acme.com",
	} {
		if answer := here.serve(vouchedRequest("GET", "/app/api/shell", token)); answer.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", name, answer.Code)
		}
	}
}

// Going in from gusto cloud at a path makes no session here, since every request says whose it is, and keeps the workspace
// the person chose to the organization's path, under a name a browser takes for it there.
func TestAtAPathGoingInKeepsNothingButTheWorkspaceChosen(t *testing.T) {
	here := atAPath(t)
	answer := here.serve(vouchedRequest("GET", "/auth/entry?workspace=main", vouchFor(t, "acme-com", "ada@acme.com", time.Now())))
	if answer.Code != http.StatusSeeOther || answer.Header().Get("Location") != "/" {
		t.Fatalf("%d %v", answer.Code, answer.Header())
	}
	cookies := answer.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "__Secure-"+workspaceCookie || cookies[0].Path != "/workspace/acme-com" || !cookies[0].Secure {
		t.Errorf("only the workspace is kept, to the path: %+v", cookies)
	}
	if refused := here.serve(vouchedRequest("GET", "/auth/entry", "")); refused.Code != http.StatusForbidden {
		t.Errorf("going in with nothing to say who: %d", refused.Code)
	}
}

// A session this server makes itself under a path, as the desktop app's is, is kept to the path. One made on an app's own
// host name is for the whole of that host.
func TestASessionIsKeptToWhereTheServerIsReached(t *testing.T) {
	here := atAPath(t)
	request := vouchedRequest("GET", "/auth/entry?t="+tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now()), "")
	session := sessionCookieIn(here.serve(request))
	if session == nil || session.Name != "__Secure-gusto" || session.Path != "/workspace/acme-com" {
		t.Errorf("under the path: %+v", session)
	}
	request = vouchedRequest("GET", "/auth/entry?t="+tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now()), "")
	request.Header.Set(AppHeader, SignApp(entryKey, "0123456789"))
	session = sessionCookieIn(here.serve(request))
	if session == nil || session.Name != secureCookieName || session.Path != "/" {
		t.Errorf("on an app's host name: %+v", session)
	}
}

func sessionCookieIn(answer *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range answer.Result().Cookies() {
		if (cookie.Name == "__Secure-gusto" || cookie.Name == secureCookieName) && cookie.Value != "" {
			return cookie
		}
	}
	return nil
}
