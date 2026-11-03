package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/roster"
)

// peopleWithTokens is a server behind a front door whose roster has ada as an admin and bob, and which takes computers' tokens.
func peopleWithTokens(t *testing.T) (*site, string, *apitoken.Store) {
	t.Helper()
	here, path := withPeople(t, roster.Roster{Members: []roster.Member{{Email: "ada@acme.com", Role: "admin"}, {Email: "bob@acme.com", Role: "runner"}}})
	store, err := apitoken.Open(filepath.Join(t.TempDir(), "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	here.server.UseTokens(store)
	return here, path, store
}

// rewrite gives the server a roster the front door wrote again, and waits until the server looks at it.
func rewrite(t *testing.T, path string, members ...roster.Member) {
	t.Helper()
	if err := roster.Write(path, roster.Roster{Members: members}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(rosterRecheck + 50*time.Millisecond)
}

func TestAPersonDemotedOrRemovedAtTheFrontDoorIsHeldToItAtTheirNextRequest(t *testing.T) {
	here, path, _ := peopleWithTokens(t)
	if page := here.ask(t, "GET", "/admin/people", nil); page.Code != http.StatusOK {
		t.Fatalf("ada, an admin, was refused her admin page: %d", page.Code)
	}
	rewrite(t, path, roster.Member{Email: "ada@acme.com", Role: "runner"}, roster.Member{Email: "bob@acme.com", Role: "admin"})
	if page := here.ask(t, "GET", "/admin/people", nil); page.Code == http.StatusOK {
		t.Error("ada, made a runner, still has her admin page")
	}
	rewrite(t, path, roster.Member{Email: "bob@acme.com", Role: "admin"})
	if page := here.ask(t, "GET", "/chat", nil); page.Code != http.StatusSeeOther || !strings.Contains(page.Header().Get("Location"), "/login") {
		t.Errorf("ada, removed, is still signed in: %d %s", page.Code, page.Header().Get("Location"))
	}
}

func TestAComputerOfAPersonWhoWasRemovedStopsWorking(t *testing.T) {
	here, path, store := peopleWithTokens(t)
	_, secret, err := store.CreateFor("ada@acme.com", "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if got := here.call("GET", "/api/me", secret); got.Code != http.StatusOK {
		t.Fatalf("ada's computer was refused: %d", got.Code)
	}
	rewrite(t, path, roster.Member{Email: "bob@acme.com", Role: "admin"})
	if got := here.call("GET", "/api/me", secret); got.Code != http.StatusUnauthorized {
		t.Errorf("the computer of a person who was removed still works: %d", got.Code)
	}
}

func TestSigningOutEverywhereEndsEveryBrowserAndEveryComputer(t *testing.T) {
	here, _, store := peopleWithTokens(t)
	// A session is stamped with the millisecond it began, and one that began in the millisecond of a cut is ended by it.
	first := here.cookie
	time.Sleep(5 * time.Millisecond)
	second := cookieOf(t, here.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	_, secret, err := store.CreateFor("ada@acme.com", "laptop")
	if err != nil {
		t.Fatal(err)
	}
	here.cookie = second
	if got := here.ask(t, "POST", "/device/sign-out-everywhere", url.Values{}); got.Code != http.StatusSeeOther {
		t.Fatalf("signing out everywhere answered %d", got.Code)
	}
	for name, cookie := range map[string]*http.Cookie{"the first browser": first, "the second": second} {
		here.cookie = cookie
		if page := here.ask(t, "GET", "/chat", nil); page.Code != http.StatusSeeOther {
			t.Errorf("%s is still signed in: %d", name, page.Code)
		}
	}
	if got := here.call("GET", "/api/me", secret); got.Code != http.StatusUnauthorized {
		t.Errorf("ada's computer still works: %d", got.Code)
	}
	time.Sleep(5 * time.Millisecond)
	here.cookie = cookieOf(t, here.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	if page := here.ask(t, "GET", "/chat", nil); page.Code != http.StatusOK {
		t.Errorf("a sign-in after signing out everywhere does not work: %d", page.Code)
	}
}

func TestAComputerWhoseTokenCouldNotBeMadeAsksAgain(t *testing.T) {
	here := withTokens(t)
	started := jsonOf(t, here.post("/api/device", `{"label":"Laptop"}`, ""))
	device, user := started["device_code"].(string), started["user_code"].(string)
	here.as(t, "ada@acme.com").ask(t, "POST", "/device", url.Values{"code": {user}})
	// A directory where the tokens' file goes makes the token fail to be made.
	away := here.file + ".away"
	if err := os.Rename(here.file, away); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(here.file, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := here.post("/api/device/token", `{"device_code":"`+device+`"}`, ""); got.Code != http.StatusInternalServerError {
		t.Fatalf("a token that could not be made answered %d %s", got.Code, got.Body)
	}
	if err := os.Remove(here.file); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(away, here.file); err != nil {
		t.Fatal(err)
	}
	if got := here.post("/api/device/token", `{"device_code":"`+device+`"}`, ""); got.Code != http.StatusOK {
		t.Errorf("the computer asked again and was given %d %s", got.Code, got.Body)
	}
}

func TestApprovingCodesThatAreNotWaitingIsCountedAsWrongAttempts(t *testing.T) {
	here := withTokens(t)
	ada := here.as(t, "ada@acme.com")
	for range failuresAllowed {
		ada.ask(t, "POST", "/device", url.Values{"code": {"BCDF-GHJK"}})
	}
	started := jsonOf(t, here.post("/api/device", `{"label":"Laptop"}`, ""))
	if got := ada.ask(t, "POST", "/device", url.Values{"code": {started["user_code"].(string)}}); strings.Contains(got.Body.String(), "Approved <strong>") {
		t.Error("a source that guessed codes went on guessing")
	}
}
