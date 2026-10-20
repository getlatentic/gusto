package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/taskindex/taskindextest"
)

// listed is a server whose task list reads an index that holds every task there is, the way a server that has
// finished building it does.
type listed struct {
	*tokened
	store taskindex.Store
}

func withIndexedTasks(t *testing.T, store taskindex.Store) *listed {
	t.Helper()
	here := withTokens(t)
	here.signIn(t)
	here.server.SetTasks(nil)
	return &listed{tokened: here, store: store}
}

// attach fills the index from the tasks and gives it to the pages.
func (l *listed) attach(t *testing.T) {
	t.Helper()
	if _, err := l.index.Reconcile(t.Context(), l.store, nil); err != nil {
		t.Fatal(err)
	}
	l.server.SetTasks(l.store)
}

func (l *listed) addTask(t *testing.T, id, asked string, state jobs.State, agentType, brief string, queued int64) {
	t.Helper()
	record := jobs.Record{ID: id, State: state, Brief: brief, Harness: agentType, Asked: asked, Repo: "acme/web", Queued: queued}
	if state.Done() {
		record.Ended = queued + 60
	}
	if err := os.MkdirAll(record.Dir(l.index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := l.index.Add(record); err != nil {
		t.Fatal(err)
	}
}

// somePeoplesTasks gives alice four tasks and bob three, in every state, from three agent types.
func (l *listed) somePeoplesTasks(t *testing.T) {
	t.Helper()
	now := time.Now().Unix()
	l.addTask(t, "a-passed", alice, jobs.Passed, "codex", "fix the login form", now-100)
	l.addTask(t, "a-failed", alice, jobs.Failed, "claude-code", "migrate 100% of the users", now-200)
	l.addTask(t, "a-interrupted", alice, jobs.Interrupted, "codex", "write the docs", now-300)
	l.addTask(t, "a-queued", alice, jobs.Queued, "pi", "snake_case the names", now-50)
	l.addTask(t, "b-passed", bob, jobs.Passed, "codex", "bobs secret project", now-400)
	l.addTask(t, "b-running", bob, jobs.Running, "pi", "bobs running work", now-10)
	l.addTask(t, "b-old", bob, jobs.Failed, "claude-code", "bobs old failure", now-30*24*3600)
}

var taskLink = regexp.MustCompile(`class="[^"]*\bbrief\b[^"]*" href="/jobs/([^"]+)"`)

func idsOnPage(body string) []string {
	var ids []string
	for _, found := range taskLink.FindAllStringSubmatch(body, -1) {
		ids = append(ids, found[1])
	}
	return ids
}

var figureCounts = regexp.MustCompile(`class="is-(\w+)[^"]*" href="[^"]*"[^>]*><span class="[^"]*\bdot\b[^"]*"></span>\w+ <span class="[^"]*\bn\b[^"]*">(\d+)</span>`)

func figuresOnPage(body string) string {
	var out []string
	for _, found := range figureCounts.FindAllStringSubmatch(body, -1) {
		out = append(out, found[1]+"="+found[2])
	}
	return strings.Join(out, ",")
}

func eachStore(t *testing.T, body func(t *testing.T, store taskindex.Store)) {
	t.Helper()
	t.Run("fake", func(t *testing.T) { body(t, taskindextest.New()) })
	t.Run("sqlite", func(t *testing.T) {
		store, err := taskindex.Open(t.Context(), "", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
		body(t, store)
	})
}

// The list shows what it showed before: the same tasks, in the same order, with the same counts, to an admin and to
// each person, whether it reads memory or the index.
func TestTheListFromTheIndexShowsWhatTheListFromMemoryShows(t *testing.T) {
	eachStore(t, func(t *testing.T, store taskindex.Store) {
		here := withIndexedTasks(t, store)
		here.somePeoplesTasks(t)
		viewers := map[string]*site{"admin": here.site, "alice": here.as(t, alice), "bob": here.as(t, bob)}
		paths := []string{"/jobs", "/jobs?state=failed", "/jobs?state=passed", "/jobs?state=queued", "/jobs?harness=codex", "/jobs?agent_type=pi",
			"/jobs?state=failed&harness=claude-code", "/jobs?state=nonsense"}
		before := map[string][3]string{}
		for who, viewer := range viewers {
			for _, path := range paths {
				body := viewer.ask(t, "GET", path, nil).Body.String()
				before[who+path] = [3]string{strings.Join(idsOnPage(body), " "), figuresOnPage(body), strings.Join(harnessOptions(body), ",")}
			}
		}
		here.attach(t)
		for who, viewer := range viewers {
			for _, path := range paths {
				answer := viewer.ask(t, "GET", path, nil)
				body := answer.Body.String()
				got := [3]string{strings.Join(idsOnPage(body), " "), figuresOnPage(body), strings.Join(harnessOptions(body), ",")}
				if want := before[who+path]; got != want {
					t.Errorf("%s %s: from the index\n%q\nfrom memory\n%q", who, path, got, want)
				}
			}
		}
		if ids := idsOnPage(viewers["admin"].ask(t, "GET", "/jobs", nil).Body.String()); len(ids) != 7 {
			t.Fatalf("the admin sees %v", ids)
		}
	})
}

var optionTag = regexp.MustCompile(`<option(?: selected)?>([^<]+)</option>`)

func harnessOptions(body string) []string {
	var out []string
	for _, found := range optionTag.FindAllStringSubmatch(body, -1) {
		out = append(out, found[1])
	}
	return out
}

func TestTheListCanBeSearchedFilteredAndOrdered(t *testing.T) {
	eachStore(t, func(t *testing.T, store taskindex.Store) {
		here := withIndexedTasks(t, store)
		here.somePeoplesTasks(t)
		here.attach(t)
		for path, want := range map[string]string{
			"/jobs?q=LOGIN":                           "a-passed",
			"/jobs?q=100%25":                          "a-failed",
			"/jobs?q=snake_c":                         "a-queued",
			"/jobs?q=snake%25c":                       "",
			"/jobs?q=bob%40example":                   "b-running b-passed b-old",
			"/jobs?person=" + url.QueryEscape(bob):    "b-running b-passed b-old",
			"/jobs?person=BOB%40EXAMPLE.COM":          "b-running b-passed b-old",
			"/jobs?agent_type=pi":                     "b-running a-queued",
			"/jobs?state=failed":                      "a-failed a-interrupted b-old",
			"/jobs?state=failed&agent_type=codex":     "a-interrupted",
			"/jobs?state=running&q=work":              "b-running",
			"/jobs?sort=seq":                          "b-old b-running b-passed a-queued a-interrupted a-failed a-passed",
			"/jobs?q=work&state=passed&person=nobody": "",
		} {
			body := here.ask(t, "GET", path, nil).Body.String()
			if got := strings.Join(idsOnPage(body), " "); got != want {
				t.Errorf("%s shows %q, want %q", path, got, want)
			}
		}
		if body := here.ask(t, "GET", "/jobs?q=nothing+like+it", nil).Body.String(); !strings.Contains(body, "Nothing matches these filters") {
			t.Error("an empty search does not say so")
		}
		if body := here.ask(t, "GET", "/jobs", nil).Body.String(); !strings.Contains(body, `name="q"`) || !strings.Contains(body, "7 tasks") {
			t.Errorf("the page has no search box or no total:\n%s", body)
		}
	})
}

func TestTheListPagesByCursor(t *testing.T) {
	here := withIndexedTasks(t, taskindextest.New())
	now := time.Now().Unix()
	for n := 0; n < 120; n++ {
		here.addTask(t, fmt.Sprintf("t%03d", n), alice, jobs.Passed, "codex", fmt.Sprintf("task %d", n), now-int64(1000-n))
	}
	here.attach(t)
	var seen []string
	path := "/jobs"
	for pages := 0; pages < 10 && path != ""; pages++ {
		body := here.ask(t, "GET", path, nil).Body.String()
		seen = append(seen, idsOnPage(body)...)
		path = ""
		if found := regexp.MustCompile(`<a class="[^"]*" href="([^"]*after=[^"]*)">Older</a>`).FindStringSubmatch(body); found != nil {
			path = strings.ReplaceAll(found[1], "&amp;", "&")
		}
	}
	if len(seen) != 120 || seen[0] != "t119" || seen[119] != "t000" {
		t.Fatalf("the pages showed %d tasks, from %v", len(seen), seen[:3])
	}
	second := here.ask(t, "GET", "/jobs", nil).Body.String()
	if !strings.Contains(second, "Older") || strings.Contains(second, ">Newest</a>") {
		t.Error("the first page has no way on, or a way back to where it is")
	}
	if got := here.ask(t, "GET", "/jobs?after=garbage", nil); got.Code != http.StatusBadRequest {
		t.Errorf("a cursor that is rubbish answered %d", got.Code)
	}
	queuedCursor := regexp.MustCompile(`after=([^"&]+)`).FindStringSubmatch(second)[1]
	if got := here.ask(t, "GET", "/jobs?sort=seq&after="+queuedCursor, nil); got.Code != http.StatusBadRequest {
		t.Errorf("a cursor of another order answered %d", got.Code)
	}
}

func (l *listed) person(t *testing.T, who string) string {
	t.Helper()
	_, secret, err := l.server.tokens.CreateFor(who, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func (l *listed) tasksOf(t *testing.T, secret, query string) (int, tasksAnswer, string) {
	t.Helper()
	got := l.call("GET", "/api/jobs"+query, secret)
	var answer tasksAnswer
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	return got.Code, answer, got.Body.String()
}

func TestTheTasksApiListsAPersonsOwnTasksANewPageAtATime(t *testing.T) {
	here := withIndexedTasks(t, taskindextest.New())
	here.somePeoplesTasks(t)
	here.attach(t)
	secret := here.person(t, alice)
	code, answer, _ := here.tasksOf(t, secret, "?limit=3")
	if code != http.StatusOK || len(answer.Tasks) != 3 || answer.Next == "" || answer.Tasks[0].ID != "a-queued" {
		t.Fatalf("first page = %d %+v", code, answer)
	}
	if answer.Counts["passed"] != 1 || answer.Counts["failed"] != 1 || answer.Counts["interrupted"] != 1 || answer.Counts["queued"] != 1 {
		t.Fatalf("counts = %v", answer.Counts)
	}
	_, last, _ := here.tasksOf(t, secret, "?limit=3&after="+string(answer.Next))
	if len(last.Tasks) != 1 || last.Tasks[0].ID != "a-interrupted" || last.Next != "" {
		t.Fatalf("last page = %+v", last)
	}
	if _, filtered, _ := here.tasksOf(t, secret, "?state=failed,interrupted&agent_type=codex&q=docs"); len(filtered.Tasks) != 1 || filtered.Tasks[0].ID != "a-interrupted" {
		t.Fatalf("filtered = %+v", filtered)
	}
	for query, want := range map[string]int{"?limit=0": 400, "?limit=1001": 400, "?limit=1000": 200, "?sort=brief": 400, "?state=bad": 400,
		"?from=yesterday": 400, "?after=%21%21": 400, "?sort=ended": 200, "?from=2000-01-01&to=2100-01-01": 200} {
		if code, _, _ := here.tasksOf(t, secret, query); code != want {
			t.Errorf("%s answered %d, want %d", query, code, want)
		}
	}
	if _, empty, body := here.tasksOf(t, here.person(t, "nobody@example.com"), ""); empty.Tasks == nil || !strings.Contains(body, `"tasks":[]`) {
		t.Errorf("a person with no task is answered %s", body)
	}
}

// Nobody who is not an admin is shown another person's task, however the request is written: not on the list, not in
// the search, not in a filter, not in a count, and not by the API.
func TestNoParameterWidensWhatAPersonSeesOfTheTaskList(t *testing.T) {
	eachStore(t, func(t *testing.T, store taskindex.Store) {
		here := withIndexedTasks(t, store)
		here.somePeoplesTasks(t)
		here.attach(t)
		hers, secret := here.as(t, alice), here.person(t, alice)
		bobs := []string{"b-passed", "b-running", "b-old", "bobs secret project", "bobs running work", "bobs old failure", bob}
		for _, query := range []string{"", "?person=" + url.QueryEscape(bob), "?person=", "?person=%20", "?q=bob", "?q=secret", "?q=" + bob,
			"?agent_type=pi", "?state=running", "?state=failed", "?sort=seq", "?sort=ended", "?limit=1000", "?from=2000-01-01"} {
			page := hers.ask(t, "GET", "/jobs"+query, nil)
			if page.Code != http.StatusOK {
				t.Fatalf("/jobs%s answered %d", query, page.Code)
			}
			for _, word := range bobs {
				if strings.Contains(strings.ReplaceAll(page.Body.String(), "value=\""+bob+"\"", ""), word) && !strings.Contains(query, word) {
					t.Errorf("/jobs%s shows %q to alice", query, word)
				}
			}
			_, _, body := here.tasksOf(t, secret, query)
			for _, word := range bobs {
				if strings.Contains(body, word) {
					t.Errorf("/api/jobs%s shows %q to alice: %s", query, word, body)
				}
			}
		}
		if body := hers.ask(t, "GET", "/jobs", nil).Body.String(); strings.Contains(body, `name="person"`) {
			t.Error("a person who sees only their own has no use for a choice of people")
		}
		if body := here.ask(t, "GET", "/jobs", nil).Body.String(); !strings.Contains(body, `name="person"`) {
			t.Error("an admin has no way to choose a person")
		}
	})
}

// Work with no owner, or the shared sign-in's, is an admin's alone, as it is on every other page.
func TestTasksWithNoOwnerAreAnAdminsAloneOnTheList(t *testing.T) {
	eachStore(t, func(t *testing.T, store taskindex.Store) {
		here := withIndexedTasks(t, store)
		here.addTask(t, "nobodys", "", jobs.Passed, "codex", "no owner", time.Now().Unix())
		here.addTask(t, "shared", sharedSignIn, jobs.Passed, "codex", "the shared sign-in", time.Now().Unix())
		here.addTask(t, "alices", alice, jobs.Passed, "codex", "alice", time.Now().Unix())
		for _, memory := range []bool{true, false} {
			if !memory {
				here.attach(t)
			}
			hers := idsOnPage(here.as(t, alice).ask(t, "GET", "/jobs", nil).Body.String())
			all := idsOnPage(here.ask(t, "GET", "/jobs", nil).Body.String())
			if fmt.Sprint(hers) != "[alices]" || len(all) != 3 {
				t.Errorf("memory=%v: alice sees %v, the admin %v", memory, hers, all)
			}
		}
	})
}

func TestAPageSaysWhileTheIndexIsStillBeingBuilt(t *testing.T) {
	here := withIndexedTasks(t, taskindextest.New())
	here.somePeoplesTasks(t)
	here.attach(t)
	here.server.SetTasksStatus(func() taskindex.Status { return taskindex.Status{Building: true, Done: 3, Total: 12} })
	if body := here.ask(t, "GET", "/jobs", nil).Body.String(); !strings.Contains(body, "The index is still being built, 3 of 12 tasks") {
		t.Errorf("the page does not say it is still being built:\n%s", body)
	}
	if got := here.call("GET", "/api/jobs", here.person(t, alice)); got.Header().Get("Task-Index-Building") != "3 of 12 tasks" {
		t.Errorf("header = %q", got.Header().Get("Task-Index-Building"))
	}
	here.server.SetTasksStatus(func() taskindex.Status { return taskindex.Status{Done: 12, Total: 12} })
	if body := here.ask(t, "GET", "/jobs", nil).Body.String(); strings.Contains(body, "still being built") {
		t.Error("a finished index is said to be building")
	}
}

func TestWithoutAnIndexThePageIsAsItWasAndTheApiSaysThereIsNone(t *testing.T) {
	here := withIndexedTasks(t, taskindextest.New())
	here.somePeoplesTasks(t)
	page := here.ask(t, "GET", "/jobs", nil)
	if page.Code != http.StatusOK || len(idsOnPage(page.Body.String())) != 7 || strings.Contains(page.Body.String(), `name="q"`) {
		t.Fatalf("the page = %d with %v", page.Code, idsOnPage(page.Body.String()))
	}
	if code, _, _ := here.tasksOf(t, here.person(t, alice), ""); code != http.StatusNotFound {
		t.Errorf("the API answered %d", code)
	}
}

func TestAnIndexThatCannotBeReadIsAServerError(t *testing.T) {
	here := withIndexedTasks(t, taskindextest.New())
	here.somePeoplesTasks(t)
	here.attach(t)
	here.store.Close()
	closed := &failing{here.store}
	here.server.SetTasks(closed)
	if page := here.ask(t, "GET", "/jobs", nil); page.Code != http.StatusInternalServerError || strings.Contains(page.Body.String(), "hunter2") {
		t.Fatalf("page = %d %s", page.Code, page.Body)
	}
	if code, _, body := here.tasksOf(t, here.person(t, alice), ""); code != http.StatusInternalServerError || strings.Contains(body, "hunter2") {
		t.Fatalf("api = %d %s", code, body)
	}
}

type failing struct{ taskindex.Store }

func (failing) List(_ context.Context, _ taskindex.Filter, _ taskindex.Page) ([]taskindex.Row, taskindex.Cursor, error) {
	return nil, "", fmt.Errorf("could not reach postgres://ada:hunter2@db/x")
}
