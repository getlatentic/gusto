package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/jobs"
)

const (
	alice = "alice@example.com"
	bob   = "bob@example.com"
)

// as is the site seen by somebody a provider signed in by address.
func (s *site) as(t *testing.T, who string) *site {
	t.Helper()
	role, _ := s.server.roleFor(Person{Email: who})
	issued := httptest.NewRecorder()
	s.server.keeper.issueFor(issued, httptest.NewRequest("GET", "/auth/callback", nil), who, role)
	seen := *s
	for _, cookie := range issued.Result().Cookies() {
		if cookie.Name == cookieName {
			seen.cookie = cookie
		}
	}
	return &seen
}

func started(t *testing.T, who *site) (instance, thread string) {
	t.Helper()
	thread = startedThread(t, who)
	return thread[:strings.Index(thread, "/threads/")], thread
}

// Somebody else's agent answers as a missing one does, on every page that
// takes it, so nothing about it can be read or changed by guessing its address.
func TestSomebodyElsesAgentIsNotFoundOnAnyPage(t *testing.T) {
	site := open(t)
	mine, theirs := site.as(t, alice), site.as(t, bob)
	where, thread := started(t, mine)

	for _, page := range []struct{ method, path string }{
		{"GET", where},
		{"GET", where + "/files"},
		{"GET", where + "/file?path=notes.txt"},
		{"GET", thread},
		{"GET", thread + "/turns"},
		{"POST", thread + "/say"},
		{"POST", where + "/tasks"},
		{"POST", where + "/threads"},
		{"POST", where + "/remove"},
	} {
		if got := theirs.ask(t, page.method, page.path, url.Values{"said": {"hello"}, "brief": {"x"}, "gate": {"true"}}); got.Code != http.StatusNotFound {
			t.Errorf("%s %s answered %d to somebody else", page.method, page.path, got.Code)
		}
	}
	if got := mine.ask(t, "GET", where, nil); got.Code != http.StatusOK {
		t.Errorf("the owner's own agent answered %d", got.Code)
	}
	if _, found := site.server.instances.Get(strings.TrimPrefix(where, "/instances/")); !found {
		t.Error("somebody else removed the agent")
	}
}

func TestTheHomePageListsOnlyYourOwnAgents(t *testing.T) {
	site := open(t)
	hers, _ := started(t, site.as(t, alice))
	his, _ := started(t, site.as(t, bob))

	page := site.as(t, alice).ask(t, "GET", "/", nil).Body.String()
	if !strings.Contains(page, hers) || strings.Contains(page, his) {
		t.Errorf("alice's home page should list %s and not %s", hers, his)
	}
}

func TestAnAdminAndTheSharedSignInSeeEverybodysAgents(t *testing.T) {
	here := open(t)
	here.server.UseAdmins([]string{" Carol@Example.com "})
	where, _ := started(t, here.as(t, alice))

	here.signIn(t)
	for name, seen := range map[string]*site{"the shared sign-in": here, "a named admin": here.as(t, "carol@example.com")} {
		if got := seen.ask(t, "GET", where, nil); got.Code != http.StatusOK {
			t.Errorf("%s got %d for somebody's agent", name, got.Code)
		}
	}
	if got := here.as(t, bob).ask(t, "GET", where, nil); got.Code != http.StatusNotFound {
		t.Errorf("a person who is not an admin got %d", got.Code)
	}
}

// A run is the asker's, and so is everything it left behind: the page, each
// file, the bundle and the live stream.
func TestSomebodyElsesJobIsNotFoundOnAnyPage(t *testing.T) {
	site := open(t)
	mine, theirs := site.as(t, alice), site.as(t, bob)
	where, _ := started(t, mine)
	task := mine.ask(t, "POST", where+"/tasks", url.Values{"brief": {"count the files"}, "gate": {"true"}})
	job := task.Header().Get("Location")
	id := strings.TrimPrefix(job, "/jobs/")
	if id == job {
		t.Fatalf("the task did not open a job: %d %q", task.Code, job)
	}

	for _, path := range []string{job, job + "/files/transcript.log", job + "/bundle", "/watch/" + id} {
		if got := theirs.ask(t, "GET", path, nil); got.Code != http.StatusNotFound {
			t.Errorf("GET %s answered %d to somebody else", path, got.Code)
		}
	}
	if got := mine.ask(t, "GET", job, nil); got.Code != http.StatusOK {
		t.Errorf("the asker's own job answered %d", got.Code)
	}
	if list := theirs.ask(t, "GET", "/jobs", nil).Body.String(); strings.Contains(list, id) {
		t.Error("somebody else's job is on the jobs list")
	}
	if list := mine.ask(t, "GET", "/jobs", nil).Body.String(); !strings.Contains(list, id) {
		t.Error("your own job is missing from the jobs list")
	}
}

// Work the shared sign-in made, and work from before anything was recorded,
// has no person to belong to, so a person is not shown it.
func TestWorkWithNoOwnerIsAnAdminsAlone(t *testing.T) {
	site := open(t)
	for id, asked := range map[string]string{"asked-by-nobody": "", "asked-by-the-shared-sign-in": sharedSignIn} {
		spentJob(t, site.index.Root(), id, "platform", priced)
		if err := site.index.Add(jobs.Record{ID: id, State: jobs.Passed, Brief: "old", Harness: "fixture", Asked: asked}); err != nil {
			t.Fatal(err)
		}
		if got := site.as(t, alice).ask(t, "GET", "/jobs/"+id, nil); got.Code != http.StatusNotFound {
			t.Errorf("a job asked by %q answered %d to a person", asked, got.Code)
		}
		site.signIn(t)
		if got := site.ask(t, "GET", "/jobs/"+id, nil); got.Code != http.StatusOK {
			t.Errorf("a job asked by %q answered %d to the shared sign-in", asked, got.Code)
		}
	}
}

func TestSomebodyElsesComparisonIsNotFound(t *testing.T) {
	site := open(t)
	board, err := compare.OpenBoard(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site.server.UseBoard(board)
	if err := board.Save(compare.Lineup{ID: "one-lineup", Brief: "fix the parser", Who: alice}); err != nil {
		t.Fatal(err)
	}

	if got := site.as(t, bob).ask(t, "GET", "/compare/one-lineup", nil); got.Code != http.StatusNotFound {
		t.Errorf("somebody else's comparison answered %d", got.Code)
	}
	if list := site.as(t, bob).ask(t, "GET", "/compare", nil).Body.String(); strings.Contains(list, "fix the parser") {
		t.Error("somebody else's comparison is on the compare page")
	}
	if got := site.as(t, alice).ask(t, "GET", "/compare/one-lineup", nil); got.Code != http.StatusOK {
		t.Errorf("the owner's own comparison answered %d", got.Code)
	}
}

// Each person sees what their own runs cost. The shared sign-in and an admin
// see the platform's whole bill, and so does a script with the password.
func TestSpendIsYourOwnUnlessYouAreAnAdmin(t *testing.T) {
	here := open(t)
	here.server.UseAdmins([]string{"carol@example.com"})
	for id, asked := range map[string]string{"alice-run": alice, "bob-run": bob} {
		spentJob(t, here.index.Root(), id, "platform", priced)
		if err := here.index.Add(jobs.Record{ID: id, State: jobs.Passed, Brief: "b", Harness: "fixture", Asked: asked}); err != nil {
			t.Fatal(err)
		}
	}
	// Each run cost $0.25, so a person who sees only their own sees $0.2500 and one who sees the bill sees $0.5000.
	spent := func(who *site) string {
		body := who.ask(t, "GET", "/spend?month=2026-09", nil).Body.String()
		switch {
		case strings.Contains(body, "$0.5000"):
			return "both"
		case strings.Contains(body, "$0.2500"):
			return "one"
		}
		return "none"
	}

	if got := spent(here.as(t, alice)); got != "one" {
		t.Errorf("alice should see her own run's cost and not bob's, saw %s", got)
	}
	if got := spent(here.as(t, "carol@example.com")); got != "both" {
		t.Errorf("an admin should see every run's cost, saw %s", got)
	}
	here.signIn(t)
	if got := spent(here); got != "both" {
		t.Errorf("the shared sign-in should see every run's cost, saw %s", got)
	}
	script := httptest.NewRequest("GET", "/api/spend?month=2026-09", nil)
	script.Header.Set("Authorization", "Bearer "+password)
	answer := httptest.NewRecorder()
	here.handler.ServeHTTP(answer, script)
	if body := answer.Body.String(); !strings.Contains(body, "alice-run") || !strings.Contains(body, "bob-run") {
		t.Errorf("a script with the password should see the whole report: %s", body)
	}
}
