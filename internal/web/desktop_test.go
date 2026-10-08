package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheServerHandsOutTheDesktopAppsReleasesAndNothingElse(t *testing.T) {
	here := open(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "latest.json"), []byte(`{"version":"1.2.3"}`), 0o644)
	os.MkdirAll(filepath.Join(dir, "darwin"), 0o755)
	os.WriteFile(filepath.Join(dir, "darwin", "app.tar.gz"), []byte("archive"), 0o644)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(secret, []byte("hunter2"), 0o644)
	here.server.UseDesktop(dir)
	handler := here.server.Handler()

	get := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		return recorder
	}
	if got := get("/desktop/latest.json"); got.Code != http.StatusOK || got.Body.String() != `{"version":"1.2.3"}` || got.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("latest.json: %d %q %q", got.Code, got.Body.String(), got.Header().Get("Cache-Control"))
	}
	if got := get("/desktop/darwin/app.tar.gz"); got.Code != http.StatusOK || got.Body.String() != "archive" {
		t.Errorf("an update: %d %q", got.Code, got.Body.String())
	}
	for _, path := range []string{"/desktop/", "/desktop/darwin/", "/desktop/darwin", "/desktop/nothing", "/desktop/%2e%2e/" + filepath.Base(secret), "/desktop/..%2f..%2f" + secret, "/desktop/%2e%2e%2f%2e%2e%2fetc/passwd"} {
		got := get(path)
		if got.Code == http.StatusOK || strings.Contains(got.Body.String(), "hunter2") || strings.Contains(got.Body.String(), "app.tar.gz") {
			t.Errorf("%s answered %d with %q: a folder was listed or a file outside it was served", path, got.Code, got.Body.String())
		}
	}
	if post := httptest.NewRecorder(); func() bool {
		handler.ServeHTTP(post, httptest.NewRequest("POST", "/desktop/latest.json", strings.NewReader("x")))
		return post.Code == http.StatusOK
	}() {
		t.Error("something was written to the releases folder through the server")
	}
}

func TestAServerWithNoReleasesFolderHandsOutNothingAtThatAddress(t *testing.T) {
	here := open(t)
	recorder := httptest.NewRecorder()
	here.server.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/desktop/latest.json", nil))
	if recorder.Code == http.StatusOK {
		t.Errorf("answered %d", recorder.Code)
	}
}
