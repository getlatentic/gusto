package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func signedInTo(t *testing.T, k *keeper, who string) *httptest.ResponseRecorder {
	t.Helper()
	recorded := httptest.NewRecorder()
	k.issueFor(recorded, httptest.NewRequest("GET", "/", nil), who, Runner)
	return recorded
}

func askWith(k *keeper, recorded *httptest.ResponseRecorder) bool {
	request := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range recorded.Result().Cookies() {
		request.AddCookie(cookie)
	}
	return k.knows(request)
}

// A sign-in outlives a restart, and a session somebody ended stays ended after one.
func TestASignInOutlivesARestartAndSigningOutDoes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keeper.json")
	first, _ := newKeeper("shared")
	if err := first.keep(file); err != nil {
		t.Fatal(err)
	}
	kept, ended := signedInTo(t, first, "ada@example.com"), signedInTo(t, first, "bob@example.com")

	endRequest := httptest.NewRequest("POST", "/logout", nil)
	for _, cookie := range ended.Result().Cookies() {
		endRequest.AddCookie(cookie)
	}
	first.end(endRequest)

	second, _ := newKeeper("shared")
	if err := second.keep(file); err != nil {
		t.Fatal(err)
	}
	if !askWith(second, kept) {
		t.Error("a sign-in did not survive a restart")
	}
	if askWith(second, ended) {
		t.Error("a session somebody ended worked again after a restart")
	}
}

// A file that is not readable as what the keeper keeps is replaced, and never stops the server.
func TestAKeeperFileThatIsNotOneIsReplaced(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keeper.json")
	if err := os.WriteFile(file, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	k, _ := newKeeper("shared")
	if err := k.keep(file); err != nil {
		t.Fatal(err)
	}
	again, _ := newKeeper("shared")
	again.keep(file)
	if !askWith(again, signedInTo(t, k, "ada@example.com")) {
		t.Error("the replaced key was not kept")
	}
}
