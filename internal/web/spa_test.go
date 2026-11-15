package web

import (
	"net/http"
	"strings"
	"testing"
)

// The console's pages that moved to the React app are its one page, behind the console's own sign-in, and so is their data.
func TestTheAppsPagesAreBehindTheConsolesSignIn(t *testing.T) {
	here := everything(t)
	admin, runner := here.as(t, boss), here.as(t, alice)
	for _, path := range []string{"/admin", "/admin/spending", "/admin/usage", "/admin/people"} {
		page := admin.ask(t, "GET", path, nil)
		body := page.Body.String()
		if page.Code != http.StatusOK || !strings.Contains(body, `<div id="root">`) || !strings.Contains(body, "/app/assets/") {
			t.Errorf("%s is not the app: %d\n%s", path, page.Code, body)
		}
		if !strings.Contains(body, "/static/look.js?v=") || strings.Contains(body, "<!--gusto-head-->") {
			t.Errorf("%s does not load the tokens and the theme in its head:\n%s", path, body)
		}
		if got := runner.ask(t, "GET", path, nil); got.Code != http.StatusForbidden {
			t.Errorf("a runner asking for %s got %d", path, got.Code)
		}
		if got := here.site.ask(t, "GET", path, nil); got.Header().Get("Location") != "/login" {
			t.Errorf("a stranger asking for %s was sent to %q", path, got.Header().Get("Location"))
		}
	}
	for _, path := range []string{"/app/api/console", "/app/api/admin", "/app/api/admin/spending", "/app/api/admin/usage", "/app/api/admin/people"} {
		if got := admin.ask(t, "GET", path, nil); got.Code != http.StatusOK || !strings.HasPrefix(got.Header().Get("Content-Type"), "application/json") {
			t.Errorf("an admin asking for %s got %d %s", path, got.Code, got.Header().Get("Content-Type"))
		}
		if got := runner.ask(t, "GET", path, nil); got.Code != http.StatusForbidden {
			t.Errorf("a runner asking for %s got %d", path, got.Code)
		}
	}
}

// The app's built files are named by their content, so a browser may keep them; its page and its data it may not.
func TestTheAppsBuiltFilesAreKeptAndItsPageIsNot(t *testing.T) {
	admin := everything(t).as(t, boss)
	page := admin.ask(t, "GET", "/admin", nil)
	if got := page.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("the app's page is kept: %q", got)
	}
	entries, err := spaFiles.ReadDir("spa/dist/assets")
	if err != nil || len(entries) == 0 {
		t.Fatalf("the app was not built into the binary: %v", err)
	}
	asset := admin.ask(t, "GET", "/app/assets/"+entries[0].Name(), nil)
	if asset.Code != http.StatusOK || asset.Header().Get("Cache-Control") == "no-store" {
		t.Errorf("a built file answered %d, kept as %q", asset.Code, asset.Header().Get("Cache-Control"))
	}
}
