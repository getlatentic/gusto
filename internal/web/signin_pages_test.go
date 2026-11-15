package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The sign-in page is the React app's, served to whoever is not signed in yet with the same headers as every page, and it loads
// nothing a session gives. Its data is open, and kept by nobody.
func TestTheSignInPageIsTheAppAndNeedsNoSession(t *testing.T) {
	stranger := open(t)
	page := stranger.ask(t, "GET", "/login", nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `<div id="root">`) || !strings.Contains(body, "/static/look.js?v=") {
		t.Fatalf("the sign-in page is not the app: %d\n%s", page.Code, body)
	}
	if page.Header().Get("Cache-Control") != "no-store" || page.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("the sign-in page may be kept or framed: %v", page.Header())
	}
	nonce := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(page.Header().Get("Content-Security-Policy"))
	if nonce == nil {
		t.Fatal("the sign-in page's policy carries no nonce")
	}
	for _, tag := range inlineScript.FindAllStringSubmatch(body, -1) {
		if !strings.Contains(tag[1], "src=") && !strings.Contains(tag[1], `nonce="`+nonce[1]+`"`) {
			t.Errorf("an inline script would not run: <script%s>", tag[1])
		}
	}
	if handler := regexp.MustCompile(`\son[a-z]+=`).FindString(body); handler != "" {
		t.Errorf("an element carries a handler the policy refuses:%s", handler)
	}
	data := stranger.ask(t, "GET", "/app/api/login", nil)
	if data.Code != http.StatusOK || data.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(data.Header().Get("Content-Type"), "application/json") {
		t.Errorf("the sign-in page's data answered a stranger %d, kept as %q", data.Code, data.Header().Get("Cache-Control"))
	}
	if strings.Contains(data.Body.String(), password) {
		t.Error("the sign-in page's data carries the shared password")
	}
	stranger.signIn(t)
	if again := stranger.ask(t, "GET", "/login", nil); again.Code != http.StatusSeeOther || again.Header().Get("Location") != "/" {
		t.Errorf("somebody signed in who opens the sign-in page got %d to %q", again.Code, again.Header().Get("Location"))
	}
}

// The sign-in page shows a refusal only when this server sealed it, so a link somebody else made cannot put words on it.
func TestTheSignInPageShowsOnlyARefusalThisServerSealed(t *testing.T) {
	site := open(t)
	sealed := site.server.withMessage("/login", "refused", "that sign-in expired; try again")
	if page := signInOf(t, site, strings.TrimPrefix(sealed, "/login")); page.Refused != "that sign-in expired; try again" {
		t.Errorf("a sealed refusal is not said: %q", page.Refused)
	}
	forged := "?" + url.Values{"refused": {"call this number"}, "refused-seal": {"forged"}}.Encode()
	if page := signInOf(t, site, forged); page.Refused != "" {
		t.Errorf("a refusal this server did not seal is said: %q", page.Refused)
	}
}

// A server with a shared password and no provider opens its form at once; the desktop app's sign-in posts to the same form.
func TestTheSignInPageOffersTheSharedPasswordWhereThereIsNoProvider(t *testing.T) {
	site := open(t)
	if page := signInOf(t, site, ""); !page.Shared || page.Provider || page.Identity || page.Desktop || page.Wrong {
		t.Errorf("the sign-in page of a server with only a shared password offers %+v", page)
	}
	if page := signInOf(t, site, "?desktop=1"); !page.Desktop || !page.Shared {
		t.Errorf("the desktop app's sign-in offers %+v", page)
	}
}

// Behind gusto cloud the sign-in page is only the desktop app's, which continues in the person's browser, and it loads no lease
// banner, since nobody is signed in to extend anything.
func TestBehindGustoCloudTheDesktopsSignInContinuesInTheBrowser(t *testing.T) {
	s := behindCloud(t)
	page := s.ask(t, "GET", "/login?desktop", nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<div id="root">`) {
		t.Fatalf("the desktop app's sign-in is not the app: %d", page.Code)
	}
	if strings.Contains(page.Body.String(), "lease.js") {
		t.Error("the sign-in page loads the lease banner")
	}
	if got := signInOf(t, s, "?desktop"); !got.Desktop || !got.Provider || got.Shared || got.Identity {
		t.Errorf("the desktop app's sign-in behind gusto cloud offers %+v", got)
	}
}
