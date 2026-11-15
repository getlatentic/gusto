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

	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/taskindex/taskindextest"
	"github.com/getlatentic/gusto/internal/tasks"
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

func (l *listed) addTask(t *testing.T, id, asked string, state tasks.State, agentType, brief string, queued int64) {
	t.Helper()
	record := tasks.Record{ID: id, State: state, Brief: brief, Harness: agentType, Asked: asked, Repo: "acme/web", Queued: queued}
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
	l.addTask(t, "a-passed", alice, tasks.Passed, "codex", "fix the login form", now-100)
	l.addTask(t, "a-failed", alice, tasks.Failed, "claude-code", "migrate 100% of the users", now-200)
	l.addTask(t, "a-interrupted", alice, tasks.Interrupted, "codex", "write the docs", now-300)
	l.addTask(t, "a-queued", alice, tasks.Queued, "pi", "snake_case the names", now-50)
	l.addTask(t, "b-passed", bob, tasks.Passed, "codex", "bobs secret project", now-400)
	l.addTask(t, "b-running", bob, tasks.Running, "pi", "bobs running work", now-10)
	l.addTask(t, "b-old", bob, tasks.Failed, "claude-code", "bobs old failure", now-30*24*3600)
}

// listedOn is the ids Activity lists at path, in its order.
func listedOn(t *testing.T, here *site, path string) []string {
	t.Helper()
	return here.activity(t, strings.TrimPrefix(path, "/jobs")).ids()
}

// countsShown is the counts across the top of Activity that it shows: those with a count, and the one chosen.
func countsShown(board boardView) string {
	var out []string
	for _, each := range board.Figures {
		if each.Count > 0 || each.Current {
			out = append(out, fmt.Sprintf("%s=%d", each.Label, each.Count))
		}
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
				board := viewer.activity(t, strings.TrimPrefix(path, "/jobs"))
				before[who+path] = [3]string{strings.Join(board.ids(), " "), countsShown(board), strings.Join(board.Harnesses, ",")}
			}
		}
		here.attach(t)
		for who, viewer := range viewers {
			for _, path := range paths {
				board := viewer.activity(t, strings.TrimPrefix(path, "/jobs"))
				got := [3]string{strings.Join(board.ids(), " "), countsShown(board), strings.Join(board.Harnesses, ",")}
				if want := before[who+path]; got != want {
					t.Errorf("%s %s: from the index\n%q\nfrom memory\n%q", who, path, got, want)
				}
			}
		}
		if ids := listedOn(t, viewers["admin"], "/jobs"); len(ids) != 7 {
			t.Fatalf("the admin sees %v", ids)
		}
	})
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
			if got := strings.Join(listedOn(t, here.site, path), " "); got != want {
				t.Errorf("%s shows %q, want %q", path, got, want)
			}
		}
		// The page says nothing matches when a search finds nothing (web/src/domain/activity.test.ts).
		if empty := here.activity(t, "?q=nothing+like+it"); len(empty.runs()) != 0 || empty.Index == nil || empty.Index.Query != "nothing like it" {
			t.Errorf("an empty search is not given as one: %+v", empty)
		}
		if board := here.activity(t, ""); board.Index == nil || board.Shown != "7 tasks" {
			t.Errorf("the page has no search box or no total: %+v", board)
		}
	})
}

func TestTheListPagesByCursor(t *testing.T) {
	here := withIndexedTasks(t, taskindextest.New())
	now := time.Now().Unix()
	for n := 0; n < 120; n++ {
		here.addTask(t, fmt.Sprintf("t%03d", n), alice, tasks.Passed, "codex", fmt.Sprintf("task %d", n), now-int64(1000-n))
	}
	here.attach(t)
	var seen []string
	path := "/jobs"
	for pages := 0; pages < 10 && path != ""; pages++ {
		board := here.activity(t, strings.TrimPrefix(path, "/jobs"))
		seen = append(seen, board.ids()...)
		path = board.Older
		if path != "" && !strings.Contains(path, "after=") {
			t.Fatalf("the way to older tasks is %s", path)
		}
	}
	if len(seen) != 120 || seen[0] != "t119" || seen[119] != "t000" {
		t.Fatalf("the pages showed %d tasks, from %v", len(seen), seen[:3])
	}
	first := here.activity(t, "")
	if first.Older == "" || first.Index == nil || first.Index.Newest != "" {
		t.Error("the first page has no way on, or a way back to where it is")
	}
	if later := here.activity(t, strings.TrimPrefix(first.Older, "/jobs")); later.Index == nil || later.Index.Newest != "/jobs" {
		t.Errorf("a later page has no way back to the newest: %+v", later.Index)
	}
	if got := here.ask(t, "GET", "/app/api/jobs?after=garbage", nil); got.Code != http.StatusBadRequest {
		t.Errorf("a cursor that is rubbish answered %d", got.Code)
	}
	queuedCursor := regexp.MustCompile(`after=([^&]+)`).FindStringSubmatch(first.Older)[1]
	if got := here.ask(t, "GET", "/app/api/jobs?sort=seq&after="+queuedCursor, nil); got.Code != http.StatusBadRequest {
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
			page := hers.ask(t, "GET", "/app/api/jobs"+query, nil)
			if page.Code != http.StatusOK {
				t.Fatalf("/jobs%s answered %d", query, page.Code)
			}
			// The search box gives back the person searched for.
			shown := strings.ReplaceAll(page.Body.String(), `"Person":"`+bob+`"`, "")
			for _, word := range bobs {
				if strings.Contains(shown, word) && !strings.Contains(query, word) {
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
		if board := hers.activity(t, ""); board.Index == nil || board.Index.Everyone {
			t.Error("a person who sees only their own has no use for a choice of people")
		}
		if board := here.activity(t, ""); board.Index == nil || !board.Index.Everyone {
			t.Error("an admin has no way to choose a person")
		}
	})
}

// Work with no owner, or the shared sign-in's, is an admin's alone, as it is on every other page.
func TestTasksWithNoOwnerAreAnAdminsAloneOnTheList(t *testing.T) {
	eachStore(t, func(t *testing.T, store taskindex.Store) {
		here := withIndexedTasks(t, store)
		here.addTask(t, "nobodys", "", tasks.Passed, "codex", "no owner", time.Now().Unix())
		here.addTask(t, "shared", sharedSignIn, tasks.Passed, "codex", "the shared sign-in", time.Now().Unix())
		here.addTask(t, "alices", alice, tasks.Passed, "codex", "alice", time.Now().Unix())
		for _, memory := range []bool{true, false} {
			if !memory {
				here.attach(t)
			}
			hers := listedOn(t, here.as(t, alice), "/jobs")
			all := listedOn(t, here.site, "/jobs")
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
	// The page says it in words (web/src/pages/work/activity-page.test.tsx).
	if board := here.activity(t, ""); board.Index == nil || board.Index.Building != (taskindex.Status{Building: true, Done: 3, Total: 12}) {
		t.Errorf("the page does not say it is still being built: %+v", board.Index)
	}
	if got := here.call("GET", "/api/jobs", here.person(t, alice)); got.Header().Get("Task-Index-Building") != "3 of 12 tasks" {
		t.Errorf("header = %q", got.Header().Get("Task-Index-Building"))
	}
	here.server.SetTasksStatus(func() taskindex.Status { return taskindex.Status{Done: 12, Total: 12} })
	if board := here.activity(t, ""); board.Index == nil || board.Index.Building.Building {
		t.Error("a finished index is said to be building")
	}
}

func TestWithoutAnIndexThePageIsAsItWasAndTheApiSaysThereIsNone(t *testing.T) {
	here := withIndexedTasks(t, taskindextest.New())
	here.somePeoplesTasks(t)
	if board := here.activity(t, ""); len(board.runs()) != 7 || board.Index != nil {
		t.Fatalf("the page lists %v, searching %+v", board.ids(), board.Index)
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
	if page := here.ask(t, "GET", "/app/api/jobs", nil); page.Code != http.StatusInternalServerError || strings.Contains(page.Body.String(), "hunter2") {
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
