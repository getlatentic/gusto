package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
)

func shellOf(t *testing.T, here *site) personShell {
	t.Helper()
	answer := here.ask(t, "GET", "/app/api/shell", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the shell answered %d: %s", answer.Code, answer.Body)
	}
	var shell personShell
	if err := json.Unmarshal(answer.Body.Bytes(), &shell); err != nil {
		t.Fatalf("the shell is not JSON: %v", err)
	}
	return shell
}

func TestTheShellGivesTheSideListAndWhoIsSignedIn(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine := site.as(t, alice)
	if made := mine.ask(t, "POST", "/rooms", url.Values{"name": {"Launch plan"}}); made.Code != http.StatusSeeOther {
		t.Fatalf("making a room gave %d", made.Code)
	}
	shell := shellOf(t, mine)
	if shell.SignedAs.Who != alice || shell.SignedAs.Initials == "" {
		t.Errorf("the shell does not say who is signed in: %+v", shell.SignedAs)
	}
	if len(shell.Rail.Rooms) != 1 || shell.Rail.Rooms[0].Name != "Launch plan" || !shell.Rail.Rooms[0].Room {
		t.Errorf("the side list does not carry the room: %+v", shell.Rail.Rooms)
	}
	if shell.Admin || shell.Org != nil {
		t.Errorf("a runner on a server of its own is given the console or an organization: %+v", shell)
	}
}

func TestTheShellIsOnlyForSomeoneSignedIn(t *testing.T) {
	plain := open(t)
	for _, path := range []string{"/app/api/shell", "/app/api/chat"} {
		got := plain.ask(t, "GET", path, nil)
		if got.Code != http.StatusUnauthorized || len(got.Result().Cookies()) != 0 {
			t.Errorf("%s answered %d with %d cookies to nobody signed in", path, got.Code, len(got.Result().Cookies()))
		}
	}
}

// project is the side list's group for the agent's folder, and whether it lists one.
func (r railList) project(id string) (railGroup, bool) {
	for _, group := range slices.Concat(r.Recent, r.Older) {
		if group.ID == id {
			return group, true
		}
	}
	return railGroup{}, false
}

// room is the side list's room of the name, and whether it lists one.
func (r railList) room(name string) (railChat, bool) {
	for _, room := range slices.Concat(r.Direct, r.Rooms) {
		if room.Name == name {
			return room, true
		}
	}
	return railChat{}, false
}

func TestOnlyAPageLoadIsRememberedForAfterSigningIn(t *testing.T) {
	plain := open(t)
	asked := func(path, dest string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if dest != "" {
			r.Header.Set("Sec-Fetch-Dest", dest)
		}
		w := httptest.NewRecorder()
		plain.server.Handler().ServeHTTP(w, r)
		return w
	}
	if got := asked("/instances/a/threads/1/version", "empty"); got.Code != http.StatusUnauthorized || len(got.Result().Cookies()) != 0 {
		t.Errorf("a page's own check, signed out, gave %d and set %v", got.Code, got.Result().Cookies())
	}
	if got := asked("/instances/a/threads/1", "document"); got.Code != http.StatusSeeOther || len(got.Result().Cookies()) == 0 {
		t.Errorf("a page load, signed out, gave %d and remembered nothing", got.Code)
	}
}
