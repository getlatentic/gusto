package web

import (
	"crypto/sha256"
	"encoding/base64"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func challengeFor(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// get is one request made straight to the server, with the cookies given.
func get(t *testing.T, site *site, path string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("GET", path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	site.server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func cookieSet(answer *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == name && cookie.Value != "" {
			return cookie
		}
	}
	return nil
}

// ticketFrom is the ticket the handoff page links the app with.
func ticketFrom(t *testing.T, page string) string {
	t.Helper()
	const prefix = "latentic://signed-in?ticket="
	from := strings.Index(page, prefix)
	if from < 0 {
		t.Fatalf("the handoff page has no link to the app: %s", page)
	}
	ticket := page[from+len(prefix):]
	return ticket[:strings.IndexAny(ticket, `"&`)]
}

// The app sends the browser to sign in, the browser comes back to the app with a ticket, and the app trades the ticket and its secret for
// a session of its own, which is not the browser's.
func TestTheAppIsSignedInThroughTheBrowser(t *testing.T) {
	site := open(t)
	secret := "a secret only the app holds"
	start := "/auth/desktop/start?challenge=" + challengeFor(secret)
	if unsigned := get(t, site, start, nil, nil); unsigned.Header().Get("Location") != "/login" {
		t.Fatalf("a browser nobody signed in went to %q, not to sign in", unsigned.Header().Get("Location"))
	}
	site.signIn(t)
	signedIn := []*http.Cookie{site.cookie}
	started := get(t, site, start, signedIn, nil)
	remembered := cookieSet(started, desktopCookie)
	if started.Header().Get("Location") != "/auth/desktop/handoff" || remembered == nil {
		t.Fatalf("a signed-in browser went to %q with %v", started.Header().Get("Location"), remembered)
	}
	page := get(t, site, "/auth/desktop/handoff", append(signedIn, remembered), nil).Body.String()
	ticket := ticketFrom(t, page)

	redeem := func(ticket, secret string) *httptest.ResponseRecorder {
		query := url.Values{"ticket": {ticket}, "secret": {secret}}
		return get(t, site, "/auth/desktop/redeem?"+query.Encode(), nil, nil)
	}
	wrong := redeem(ticket, "another secret")
	if cookieSet(wrong, cookieName) != nil || !strings.HasPrefix(wrong.Header().Get("Location"), "/login") {
		t.Fatalf("the wrong secret got %q", wrong.Header().Get("Location"))
	}
	ticket = ticketFrom(t, get(t, site, "/auth/desktop/handoff", append(signedIn, remembered), nil).Body.String())
	right := redeem(ticket, secret)
	session := cookieSet(right, cookieName)
	if right.Header().Get("Location") != "/chat" || session == nil {
		t.Fatalf("the right secret got %q with %v", right.Header().Get("Location"), session)
	}
	if opened := get(t, site, "/chat", []*http.Cookie{session}, nil); opened.Code != http.StatusOK {
		t.Errorf("the app's session opened the chat with %d", opened.Code)
	}
	if again := redeem(ticket, secret); cookieSet(again, cookieName) != nil {
		t.Error("a ticket was taken twice")
	}
}

// Signing in after the app asked takes the browser back to the app, and signing in otherwise takes it to the pages.
func TestSigningInAfterTheAppAskedGoesBackToTheApp(t *testing.T) {
	site := open(t)
	asked := &http.Cookie{Name: desktopCookie, Value: challengeFor("secret")}
	request := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"password": {password}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(asked)
	recorder := httptest.NewRecorder()
	site.server.Handler().ServeHTTP(recorder, request)
	if recorder.Header().Get("Location") != "/auth/desktop/handoff" {
		t.Errorf("signing in after the app asked went to %q", recorder.Header().Get("Location"))
	}
	site.signIn(t)
}

// A ticket is good once, for the secret it was made for.
func TestATicketIsGoodOnceAndOnlyForItsSecret(t *testing.T) {
	var held tickets
	now := time.Now()
	id := held.mint("somebody@example.com", Runner, challengeFor("secret"), now)
	if _, _, ok := held.redeem(id, "not it", now); ok {
		t.Error("a ticket was taken with the wrong secret")
	}
	if _, _, ok := held.redeem(id, "secret", now); ok {
		t.Error("a ticket tried with the wrong secret was still good")
	}
	id = held.mint("somebody@example.com", Runner, challengeFor("secret"), now)
	if who, role, ok := held.redeem(id, "secret", now); !ok || who != "somebody@example.com" || role != Runner {
		t.Errorf("the right secret got %q %q %v", who, role, ok)
	}
	if _, _, ok := held.redeem(id, "secret", now); ok {
		t.Error("a ticket was taken twice")
	}
	id = held.mint("somebody@example.com", Runner, challengeFor("secret"), now)
	if _, _, ok := held.redeem(id, "secret", now.Add(ticketLife)); ok {
		t.Error("a ticket was taken after it expired")
	}
}

func TestOnlyAHashIsAChallenge(t *testing.T) {
	for _, bad := range []string{"", "short", strings.Repeat("!", challengeLength), strings.Repeat("a", challengeLength+1)} {
		if validChallenge(bad) {
			t.Errorf("%q was taken for a challenge", bad)
		}
	}
	if !validChallenge(challengeFor("x")) {
		t.Error("a hash was refused")
	}
}

// A request that another site sent the person to cannot sign them in as somebody else.
func TestACrossSiteRedeemIsRefused(t *testing.T) {
	site := open(t)
	answer := get(t, site, "/auth/desktop/redeem?ticket=x&secret=y", nil, map[string]string{"Sec-Fetch-Site": "cross-site"})
	if answer.Code != http.StatusForbidden {
		t.Errorf("a cross-site redeem answered %d", answer.Code)
	}
}

// Where a provider is named, the browser the app opens goes to the provider and does not stop at a page that asks for one more press.
func TestTheAppsBrowserGoesStraightToTheProvider(t *testing.T) {
	index, err := jobs.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	instances, err := instance.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(index, instances, nil, password, &Identity{provider: &oidc.Provider{}}, offered(), nil)
	if err != nil {
		t.Fatal(err)
	}
	site := &site{handler: server.Handler(), index: index, server: server}
	answer := get(t, site, "/auth/desktop/start?challenge="+challengeFor("secret"), nil, nil)
	if answer.Header().Get("Location") != "/auth/start" {
		t.Errorf("the browser went to %q", answer.Header().Get("Location"))
	}
}
