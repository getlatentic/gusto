package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/apps"
)

func TestAFolderWithAPageInItCanBePreviewedAndNothingElseCan(t *testing.T) {
	got := previewsOf([]string{"2048-app/index.html", "2048-app/script.js", "index.html", "docs/guide/index.html", "a/b.html", "../x/index.html", "a b/index.html", "2048-app/index.html"})
	want := []string{"2048-app", ".", "docs/guide"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%v, wanted %v", got, want)
	}
	for _, bad := range []string{"", "..", "../x", "/etc", "a/../b", "a b", "a;b", "$(x)", "-e", ".hidden/../x", "a'b", "a\nb"} {
		if previewFolder.MatchString(bad) {
			t.Errorf("%q can be previewed", bad)
		}
	}
}

// pagesSite is a server that shares the pages its agents build and runs no apps of their own: its agents have no computers.
func pagesSite(t *testing.T) *site {
	t.Helper()
	site := open(t)
	store, err := apps.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site.server.UseApps(store, nil)
	site.signIn(t)
	return site
}

func TestAPageTheAgentBuiltIsPreviewedFromTheConversationItWasBuiltIn(t *testing.T) {
	site := pagesSite(t)
	thread := startedThread(t, site)
	instance := thread[:strings.Index(thread, "/threads/")]

	submit := func(form url.Values) (int, string) {
		answer := site.ask(t, "POST", instance+"/apps", form)
		return answer.Code, answer.Header().Get("Location")
	}
	code, where := submit(url.Values{"preview": {"2048-app"}, "back": {thread}})
	if code != http.StatusSeeOther || where != thread+"#apps" {
		t.Fatalf("the person is sent back to the conversation: %d %q", code, where)
	}
	shared := site.server.appsOf(strings.TrimPrefix(instance, "/instances/"))
	if len(shared) != 1 || shared[0].Folder != "2048-app" || shared[0].State != apps.Live || shared[0].Command != "" {
		t.Fatalf("the page is not shared, live, with nothing run for it: %+v", shared)
	}
	if page := site.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, "Open the app") || strings.Contains(page, "Starting the preview") {
		t.Errorf("the conversation does not link to the page at once")
	}
	submit(url.Values{"preview": {"2048-app"}, "back": {thread}})
	if again := site.server.appsOf(strings.TrimPrefix(instance, "/instances/")); len(again) != 1 {
		t.Errorf("previewing the same folder again shared it again: %d", len(again))
	}
	code, where = submit(url.Values{"preview": {"../secrets"}, "back": {thread}})
	if !strings.Contains(where, "refused=") || !strings.HasPrefix(where, thread) {
		t.Errorf("a folder outside the agent's own was shared: %d %q", code, where)
	}
	for _, back := range []string{"//evil.example/x", "https://evil.example/", "/instances/other/threads/1", thread + "/../../x", thread + "?a=1"} {
		code, where := submit(url.Values{"preview": {"docs"}, "back": {back}})
		if code != http.StatusSeeOther || !strings.HasPrefix(where, instance) || strings.Contains(where, "/threads/") || strings.Contains(where, "evil") {
			t.Errorf("%q sent the person to %q", back, where)
		}
	}
}

// A page is its folder's files as they are now, and nothing outside the folder: not through a path, and not through a link
// the agent left in it.
func TestAPageIsServedFromItsFolderAndNothingElse(t *testing.T) {
	site := pagesSite(t)
	instance := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	folder := filepath.Join(site.server.instances.Workspace(instance), "site")
	os.MkdirAll(filepath.Join(folder, "parts"), 0o755)
	os.WriteFile(filepath.Join(folder, "index.html"), []byte("<h1>splitter</h1>"), 0o644)
	os.WriteFile(filepath.Join(folder, "app.js"), []byte("console.log(1)"), 0o644)
	os.WriteFile(filepath.Join(folder, "parts", "index.html"), []byte("<p>part</p>"), 0o644)
	os.WriteFile(filepath.Join(site.server.instances.Workspace(instance), "secret.txt"), []byte("TOPSECRET"), 0o644)
	os.Symlink(filepath.Join(site.server.instances.Workspace(instance), "secret.txt"), filepath.Join(folder, "leak.txt"))
	app, err := site.server.apps.NewPage(instance, "", "site")
	if err != nil {
		t.Fatal(err)
	}
	base := "/apps/" + app.ID

	page := site.ask(t, "GET", base+"/", nil)
	if page.Code != http.StatusOK || page.Body.String() != "<h1>splitter</h1>" || !strings.HasPrefix(page.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("the page: %d %q %q", page.Code, page.Header().Get("Content-Type"), page.Body)
	}
	if policy := page.Header().Get("Content-Security-Policy"); !strings.HasPrefix(policy, "sandbox allow-scripts allow-forms") {
		t.Errorf("a page under this server's own origin is not sandboxed: %q", policy)
	}
	if script := site.ask(t, "GET", base+"/app.js", nil); script.Code != http.StatusOK || !strings.Contains(script.Header().Get("Content-Type"), "javascript") {
		t.Errorf("its script: %d %q", script.Code, script.Header().Get("Content-Type"))
	}
	if part := site.ask(t, "GET", base+"/parts/", nil); part.Body.String() != "<p>part</p>" {
		t.Errorf("a folder of the page is not its index.html: %q", part.Body)
	}
	os.WriteFile(filepath.Join(folder, "index.html"), []byte("<h1>changed</h1>"), 0o644)
	if again := site.ask(t, "GET", base+"/", nil).Body.String(); again != "<h1>changed</h1>" {
		t.Errorf("the page is not what its folder holds now: %q", again)
	}
	for _, path := range []string{"/../secret.txt", "/%2e%2e/secret.txt", "/leak.txt", "/missing.html"} {
		if got := site.ask(t, "GET", base+path, nil); got.Code == http.StatusOK || strings.Contains(got.Body.String(), "TOPSECRET") {
			t.Errorf("%s answered %d: %q", path, got.Code, got.Body)
		}
	}
	if got := site.ask(t, "POST", base+"/", nil); got.Code != http.StatusMethodNotAllowed {
		t.Errorf("a post to a page answered %d", got.Code)
	}
}

// Behind a front door a page has a host name of its own, so the browser lets it keep what it saves, and nothing fences it.
func TestAPageBehindAFrontDoorIsItsOwnSite(t *testing.T) {
	site := pagesSite(t)
	site.server.UseFrontDoor(FrontDoor{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
	site.handler = site.server.Handler()
	instance := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	folder := filepath.Join(site.server.instances.Workspace(instance), "site")
	os.MkdirAll(folder, 0o755)
	os.WriteFile(filepath.Join(folder, "index.html"), []byte("<h1>splitter</h1>"), 0o644)
	app, err := site.server.apps.NewPage(instance, "", "site")
	if err != nil {
		t.Fatal(err)
	}
	if link := site.server.appLink(app); link != "https://acme-com--"+app.ID+".cloud.test/" {
		t.Errorf("the page's address is %q", link)
	}
	if stranger := site.appRequest(t, app.ID, "GET", "/"); stranger.Code != http.StatusSeeOther {
		t.Errorf("somebody not signed in on the page's host name was answered %d", stranger.Code)
	}
	got := site.appRequest(t, app.ID, "GET", "/", site.signedInTo(t, app.ID))
	if got.Code != http.StatusOK || got.Body.String() != "<h1>splitter</h1>" {
		t.Fatalf("the page at its own host name: %d %q", got.Code, got.Body)
	}
	if policy := got.Header().Values("Content-Security-Policy"); len(policy) != 0 {
		t.Errorf("a page with an origin of its own is fenced as if it had none: %v", policy)
	}
}
