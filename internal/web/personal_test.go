package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (s *site) getAs(secret, path string) *httptest.ResponseRecorder {
	return s.call("GET", path, secret)
}

func TestAPersonsTokenListsOnlyTheirOwnAgents(t *testing.T) {
	here := withTokens(t)
	hers, _ := started(t, here.as(t, alice))
	his, _ := started(t, here.as(t, bob))
	secret := signInADevice(t, here, alice)

	list := here.getAs(secret, "/api/instances")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), strings.TrimPrefix(hers, "/instances/")) || strings.Contains(list.Body.String(), strings.TrimPrefix(his, "/instances/")) {
		t.Errorf("alice was shown %d %s", list.Code, list.Body)
	}
	empty := here.getAs(signInADevice(t, here, "carol@example.com"), "/api/instances")
	if strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Errorf("somebody with no agents was shown %q, not an empty list", empty.Body)
	}
}

func TestSomebodyElsesAgentIsNotFoundOnEveryRouteAPersonsTokenReaches(t *testing.T) {
	here := withTokens(t)
	theirs, thread := started(t, here.as(t, bob))
	id := strings.TrimPrefix(theirs, "/instances/")
	threadID := thread[strings.LastIndex(thread, "/")+1:]
	secret := signInADevice(t, here, alice)
	for _, path := range []string{
		"/api/instances/" + id, "/api/instances/" + id + "/threads", "/api/instances/" + id + "/threads/" + threadID,
		"/api/instances/" + id + "/files", "/api/instances/" + id + "/file?path=notes.txt",
	} {
		if got := here.getAs(secret, path); got.Code != http.StatusNotFound {
			t.Errorf("alice's token got %d at %s", got.Code, path)
		}
	}
	bobs := signInADevice(t, here, bob)
	for _, path := range []string{"/api/instances/" + id, "/api/instances/" + id + "/threads", "/api/instances/" + id + "/threads/" + threadID, "/api/instances/" + id + "/files"} {
		if got := here.getAs(bobs, path); got.Code != http.StatusOK {
			t.Errorf("bob's own %s answered %d", path, got.Code)
		}
	}
}

func TestAThreadSaysWhatWasSaidInItAsAList(t *testing.T) {
	here := withTokens(t)
	mine, thread := started(t, here.as(t, alice))
	secret := signInADevice(t, here, alice)
	got := here.getAs(secret, "/api"+thread)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"said":[]`) {
		t.Errorf("a thread with nothing said answered %d %s", got.Code, got.Body)
	}
	_ = mine
}

func TestAWorkspaceListsItsFilesAndGivesThemWholeButStaysInsideItself(t *testing.T) {
	here := withTokens(t)
	where, _ := started(t, here.as(t, alice))
	id := strings.TrimPrefix(where, "/instances/")
	root := here.server.instances.Workspace(id)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not yours"), 0o600)
	for path, text := range map[string]string{"notes.txt": "hello world", "src/main.go": "package main", ".git/config": "internal"} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(text), 0o644)
	}
	os.Symlink(outside, filepath.Join(root, "out"))
	os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt"))
	secret := signInADevice(t, here, alice)

	list := here.getAs(secret, "/api/instances/"+id+"/files").Body.String()
	for _, want := range []string{`"path":"notes.txt"`, `"path":"src/main.go"`, `"size":11`} {
		if !strings.Contains(list, want) {
			t.Errorf("the listing lacks %s: %s", want, list)
		}
	}
	for _, not := range []string{".git", "leak.txt", "secret.txt", `"path":"out`} {
		if strings.Contains(list, not) {
			t.Errorf("the listing has %s: %s", not, list)
		}
	}
	if got := here.getAs(secret, "/api/instances/"+id+"/file?path=notes.txt"); got.Code != http.StatusOK || got.Body.String() != "hello world" {
		t.Errorf("the file came as %d %q", got.Code, got.Body)
	}
	for _, path := range []string{"../../etc/passwd", "leak.txt", "out/secret.txt", ".git/config", "missing.txt", ""} {
		if got := here.getAs(secret, "/api/instances/"+id+"/file?path="+path); got.Code != http.StatusNotFound {
			t.Errorf("%q answered %d", path, got.Code)
		}
	}
	ranged := httptest.NewRequest("GET", "/api/instances/"+id+"/file?path=notes.txt", nil)
	ranged.Header.Set("Authorization", "Bearer "+secret)
	ranged.Header.Set("Range", "bytes=6-")
	rec := httptest.NewRecorder()
	here.handler.ServeHTTP(rec, ranged)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "world" {
		t.Errorf("a range gave %d %q", rec.Code, rec.Body)
	}
}

func TestOnlyAPersonsTokenReachesThesePersonalRoutes(t *testing.T) {
	here := withTokens(t)
	for name, secret := range here.secrets {
		if got := here.getAs(secret, "/api/instances"); got.Code == http.StatusOK {
			t.Errorf("the organisation's %q token read a person's agents", name)
		}
	}
	if got := here.getAs(password, "/api/instances"); got.Code == http.StatusOK {
		t.Errorf("the shared password read a person's agents")
	}
	if got := here.getAs("", "/api/instances"); got.Code != http.StatusUnauthorized {
		t.Errorf("no credential answered %d", got.Code)
	}
}

func TestAPersonsTokenIsNeverAnAdminsEvenForAnAdmin(t *testing.T) {
	here := withTokens(t)
	here.server.UseAdmins([]string{alice})
	_, _ = started(t, here.as(t, bob))
	if page := here.as(t, alice).ask(t, "GET", "/", nil).Body.String(); !strings.Contains(page, "Admin") {
		t.Skip("the site does not mark an admin's page the way this test looks for")
	}
	secret := signInADevice(t, here, alice)
	if list := here.getAs(secret, "/api/instances"); strings.TrimSpace(list.Body.String()) != "[]" {
		t.Errorf("an admin's token read somebody else's agents: %s", list.Body)
	}
	both := httptest.NewRequest("GET", "/api/instances", nil)
	both.Header.Set("Authorization", "Bearer "+secret)
	both.AddCookie(here.as(t, alice).cookie)
	rec := httptest.NewRecorder()
	here.handler.ServeHTTP(rec, both)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("a request with a token and an admin's cookie was an admin's: %s", rec.Body)
	}
}
