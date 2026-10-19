package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
)

var noon = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func tokensOf(n int64) *int64  { return &n }
func usdOf(d float64) *float64 { return &d }

// aCall is a priced call of a session, made at the hour given on 14 October.
func aCall(task string, line int, session, person string, hour int) usage.Row {
	return usage.Row{Task: task, Line: line, At: noon.Add(time.Duration(hour) * time.Hour), Session: session, Person: person, Engine: "Claude Code",
		Agent: "Notes", Provider: "bedrock", Credential: "team-key", Model: "claude-sonnet", InputTokens: tokensOf(1000), OutputTokens: tokensOf(200),
		CachedTokens: tokensOf(400), USD: usdOf(0.25), Status: 200, TookMS: 900}
}

// withUsage is a server with tokens whose index holds the rows given.
func withUsage(t *testing.T, rows ...usage.Row) (*tokened, *usagetest.Fake) {
	t.Helper()
	here := withTokens(t)
	store := usagetest.New()
	if err := store.Add(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	here.server.SetUsage(store)
	here.signIn(t)
	return here, store
}

func bothPeople() []usage.Row {
	return []usage.Row{
		aCall("task-a", 0, "alice-session", alice, 0),
		aCall("task-a", 1, "alice-session", alice, 1),
		aCall("task-b", 0, "bob-session", bob, 2),
	}
}

func TestAnAdminSeesEveryonesSessionsAddedUp(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	page := here.ask(t, "GET", "/usage", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("%d %s", page.Code, page.Body)
	}
	for _, want := range []string{"alice-session", "bob-session", alice, bob, `href="/usage/alice-session"`, `href="/jobs/task-a"`, `href="/jobs/task-b"`,
		"$0.7500", ">3<", "3.0k", "40%", "Notes", "Claude Code", "bedrock", "claude-sonnet"} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("the page does not show %q", want)
		}
	}
}

func TestACallWithNoPriceIsShownAsNoPriceAndNeverAsFree(t *testing.T) {
	unpriced := aCall("task-c", 0, "free-looking", alice, 3)
	unpriced.USD = nil
	unpriced.InputTokens, unpriced.CachedTokens = nil, nil
	mixed := aCall("task-d", 0, "mixed", alice, 4)
	here, _ := withUsage(t, unpriced, mixed, aCall("task-d", 1, "mixed", alice, 5))
	body := here.ask(t, "GET", "/usage", nil).Body.String()
	if !strings.Contains(body, "no price set for these models") == strings.Contains(body, "$0.0000") {
		t.Error("the headline should not be a sum that looks complete")
	}
	only := here.ask(t, "GET", "/usage?q=free-looking", nil).Body.String()
	if !strings.Contains(only, `<span class="muted">no price</span>`) || strings.Contains(only, "$0.0000") || !strings.Contains(only, "no price set for these models") {
		t.Errorf("an unpriced session was shown as a cost:\n%s", only)
	}
	calls := here.ask(t, "GET", "/usage/free-looking", nil).Body.String()
	if !strings.Contains(calls, `<span class="muted">no price</span>`) || strings.Contains(calls, "$0") {
		t.Errorf("an unpriced call was shown as a cost:\n%s", calls)
	}
	partly := here.ask(t, "GET", "/usage", nil).Body.String()
	if !strings.Contains(partly, "spent, at least; 1 calls have no price") {
		t.Errorf("a total that leaves calls out should say so")
	}
}

func TestTheSessionsCanBeFilteredAndSearched(t *testing.T) {
	other := aCall("task-c", 0, "codex-session", alice, 5)
	other.Engine, other.Agent, other.Provider, other.Model = "Codex", "", "openai", "gpt-5"
	old := aCall("task-d", 0, "old-session", bob, 0)
	old.At = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	here, _ := withUsage(t, append(bothPeople(), other, old)...)
	for _, c := range []struct {
		query string
		shows []string
		hides []string
	}{
		{"agent_type=Codex", []string{"codex-session"}, []string{"alice-session", "bob-session"}},
		{"provider=openai", []string{"codex-session"}, []string{"alice-session"}},
		{"model=gpt-5", []string{"codex-session"}, []string{"bob-session"}},
		{"agent=Notes", []string{"alice-session", "bob-session"}, []string{"codex-session"}},
		{"person=" + url.QueryEscape(bob), []string{"bob-session", "old-session"}, []string{"alice-session"}},
		{"q=CODEX", []string{"codex-session"}, []string{"alice-session"}},
		{"from=2026-10-01", []string{"alice-session"}, []string{"old-session"}},
		{"to=2026-09-01", []string{"old-session"}, []string{"alice-session"}},
		{"from=2026-10-14&to=2026-10-14", []string{"alice-session", "codex-session"}, []string{"old-session"}},
		{"from=" + url.QueryEscape("2026-10-14T13:30:00Z"), []string{"codex-session"}, []string{"alice-session"}},
	} {
		body := here.ask(t, "GET", "/usage?"+c.query, nil).Body.String()
		for _, want := range c.shows {
			if !strings.Contains(body, "/usage/"+want) {
				t.Errorf("%s does not show %s", c.query, want)
			}
		}
		for _, hidden := range c.hides {
			if strings.Contains(body, "/usage/"+hidden) {
				t.Errorf("%s shows %s", c.query, hidden)
			}
		}
	}
	if got := here.ask(t, "GET", "/usage?from=yesterday", nil); got.Code != http.StatusBadRequest {
		t.Errorf("a date that is not one answered %d", got.Code)
	}
	if got := here.ask(t, "GET", "/usage?q=nothing-is-called-this", nil).Body.String(); !strings.Contains(got, "No session matches these filters") {
		t.Error("a search that finds nothing should say so")
	}
}

func TestTheSelectsOfTheFilterListWhatTheCallsHold(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	body := here.ask(t, "GET", "/usage?agent=Notes", nil).Body.String()
	for _, want := range []string{`name="person" data-autosubmit`, `name="agent_type" data-autosubmit`, `name="provider" data-autosubmit`, `name="model" data-autosubmit`, "<option selected>Notes</option>", "<option>bedrock</option>"} {
		if !strings.Contains(body, want) {
			t.Errorf("the filter lacks %q", want)
		}
	}
}

func TestTheSessionsPageThroughACursor(t *testing.T) {
	var rows []usage.Row
	for hour, id := range []string{"s-one", "s-two", "s-three", "s-four", "s-five"} {
		rows = append(rows, aCall("task-"+id, 0, id, alice, hour))
	}
	here, _ := withUsage(t, rows...)
	seen := map[string]int{}
	path := "/usage?limit=2"
	for pages := 0; path != "" && pages < 10; pages++ {
		body := here.ask(t, "GET", path, nil).Body.String()
		for _, id := range []string{"s-one", "s-two", "s-three", "s-four", "s-five"} {
			seen[id] += strings.Count(body, `href="/usage/`+id+`"`)
		}
		path = ""
		if i := strings.Index(body, `rel="next"`); i >= 0 {
			start := strings.LastIndex(body[:i], `href="`) + len(`href="`)
			path = strings.ReplaceAll(body[start:i-2], "&amp;", "&")
		}
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("%s was listed %d times across the pages", id, n)
		}
	}
	if len(seen) != 5 {
		t.Errorf("the pages listed %v", seen)
	}
}

func TestTheSessionsAreSortedAcrossPagesByTheColumnChosen(t *testing.T) {
	var rows []usage.Row
	for i, id := range []string{"s-a", "s-b", "s-c", "s-d"} {
		row := aCall("task-"+id, 0, id, alice, 10-i)
		row.USD = usdOf(float64([]int{2, 9, 1, 5}[i]))
		rows = append(rows, row)
	}
	here, _ := withUsage(t, rows...)
	order := func(path string) []string {
		var ids []string
		for pages := 0; path != "" && pages < 10; pages++ {
			body := here.ask(t, "GET", path, nil).Body.String()
			for _, id := range []string{"s-a", "s-b", "s-c", "s-d"} {
				if strings.Contains(body, `href="/usage/`+id+`"`) {
					ids = append(ids, id)
				}
			}
			path = ""
			if i := strings.Index(body, `rel="next"`); i >= 0 {
				start := strings.LastIndex(body[:i], `href="`) + len(`href="`)
				path = strings.ReplaceAll(body[start:i-2], "&amp;", "&")
			}
		}
		return ids
	}
	if got := strings.Join(order("/usage?sort=cost&limit=1"), ""); got != "s-bs-ds-as-c" {
		t.Errorf("by cost across pages: %s", got)
	}
	if got := strings.Join(order("/usage?limit=3"), ""); got != "s-as-bs-cs-d" {
		t.Errorf("by last use: %s", got)
	}
	body := here.ask(t, "GET", "/usage?sort=cost&q=s-", nil).Body.String()
	if !strings.Contains(body, `aria-sort="descending" class="num"><a href="/usage?q=s-&amp;sort=cost"`) || strings.Contains(body, "this page") {
		t.Errorf("the sorted column is not marked, or the filter is lost from its link:\n%s", body)
	}
	if !strings.Contains(body, `<a href="/usage?q=s-&amp;sort=tokens">Tokens</a>`) {
		t.Error("the other headings should set the sort and keep the filter")
	}
	api := jsonList(t, here.call("GET", "/api/usage/sessions?sort=calls&limit=2", here.secrets["finance"]), "sessions")
	if len(api) != 2 {
		t.Errorf("the API gave %d sessions", len(api))
	}
}

func TestAnUnknownSortIsRefusedAndNamesTheFour(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	for _, path := range []string{"/usage?sort=color", "/api/usage/sessions?sort=color"} {
		got := here.call("GET", path, here.secrets["finance"])
		if path[0:4] == "/usa" {
			got = here.ask(t, "GET", path, nil)
		}
		if got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "last, cost, calls or tokens") {
			t.Errorf("%s answered %d %s", path, got.Code, got.Body)
		}
	}
	if got := here.call("GET", "/api/usage/sessions?limit=1001", here.secrets["finance"]); got.Code != http.StatusBadRequest {
		t.Errorf("a limit over 1000 answered %d", got.Code)
	}
	if got := here.call("GET", "/api/usage/sessions?limit=1000", here.secrets["finance"]); got.Code != http.StatusOK {
		t.Errorf("a limit of 1000 answered %d", got.Code)
	}
}

func TestASessionsCallsAreNarrowedByTheSearchInTheStore(t *testing.T) {
	other := aCall("task-a", 1, "alice-session", alice, 1)
	other.Model = "rare-model"
	here, _ := withUsage(t, aCall("task-a", 0, "alice-session", alice, 0), other)
	if calls := jsonList(t, here.call("GET", "/api/usage/calls?session=alice-session&q=rare", here.secrets["finance"]), "calls"); len(calls) != 1 {
		t.Errorf("q left %d calls", len(calls))
	}
	if calls := jsonList(t, here.call("GET", "/api/usage/calls?session=alice-session&model=none", here.secrets["finance"]), "calls"); len(calls) != 0 {
		t.Errorf("a model filter left %d calls", len(calls))
	}
	if body := here.call("GET", "/api/usage/calls?session=alice-session&model=none", here.secrets["finance"]).Body.String(); !strings.Contains(body, `"calls":[]`) {
		t.Errorf("no calls should be []: %s", body)
	}
	page := here.ask(t, "GET", "/usage/alice-session?q=rare", nil).Body.String()
	if !strings.Contains(page, "rare-model") || strings.Contains(page, "task-a</a>") == false || strings.Count(page, "<tr>") != 2 {
		t.Errorf("the session page was not narrowed:\n%s", page)
	}
	if page := here.ask(t, "GET", "/usage/alice-session?q=zzz", nil); page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "No call of this session matches") {
		t.Errorf("a search with no match answered %d", page.Code)
	}
	if page := here.as(t, alice).ask(t, "GET", "/usage/alice-session?q=zzz", nil); page.Code != http.StatusOK {
		t.Errorf("a person's own session with no match answered %d", page.Code)
	}
}

func TestTheConsoleHasAUsagePageNextToSpending(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	page := here.ask(t, "GET", "/admin/usage", nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `<a href="/admin/usage" aria-current="page">Usage</a>`) || !strings.Contains(body, `href="/admin/spending"`) || !strings.Contains(body, `href="/admin/usage/alice-session"`) {
		t.Errorf("the console page was %d:\n%s", page.Code, body)
	}
	if got := here.ask(t, "GET", "/admin/usage/alice-session", nil); got.Code != http.StatusOK {
		t.Errorf("the console session page answered %d", got.Code)
	}
	if got := here.as(t, alice).ask(t, "GET", "/admin/usage", nil); got.Code != http.StatusForbidden {
		t.Errorf("the console usage for a runner answered %d", got.Code)
	}
}

func TestASessionListsItsCallsLatestFirst(t *testing.T) {
	refused := aCall("task-a", 1, "alice-session", alice, 1)
	refused.Refused, refused.Status, refused.USD, refused.Translated = "the team's daily cap", 429, nil, "anthropic to openai"
	here, _ := withUsage(t, aCall("task-a", 0, "alice-session", alice, 0), refused, aCall("task-z", 0, "alice-session", alice, 3))
	body := here.ask(t, "GET", "/usage/alice-session", nil).Body.String()
	for _, want := range []string{"team-key", "claude-sonnet", "bedrock", `href="/jobs/task-z"`, "the team&#39;s daily cap", "429", "anthropic to openai", "no price", "40%"} {
		if !strings.Contains(body, want) {
			t.Errorf("the session page lacks %q", want)
		}
	}
	if strings.Index(body, "task-z") > strings.Index(body, "task-a") {
		t.Error("the latest call should be first")
	}
	if got := here.ask(t, "GET", "/usage/nobody-made-this", nil); got.Code != http.StatusNotFound {
		t.Errorf("a session the index lacks answered %d", got.Code)
	}
}

func TestTheUsagePageSaysWhenThereIsNothingYet(t *testing.T) {
	here, _ := withUsage(t)
	body := here.ask(t, "GET", "/usage", nil).Body.String()
	if !strings.Contains(body, "No model call has been recorded yet") || strings.Contains(body, `class="list usage"`) {
		t.Errorf("the empty state is not shown:\n%s", body)
	}
	bare := open(t)
	bare.signIn(t)
	if body := bare.ask(t, "GET", "/usage", nil).Body.String(); !strings.Contains(body, "keeps no usage index") {
		t.Error("a server with no index should say so")
	}
}

func TestAnIndexStillBeingBuiltSaysHowFarItIs(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	building := true
	here.server.SetUsageStatus(func() usage.Status { return usage.Status{Building: building, Done: 3, Total: 12} })
	body := here.ask(t, "GET", "/usage", nil).Body.String()
	if !strings.Contains(body, "The index is still being built, 3 of 12 tasks") {
		t.Error("the page does not say the index is being built")
	}
	if got := here.call("GET", "/api/usage/totals", here.secrets["finance"]); got.Header().Get("Usage-Index-Building") != "3 of 12 tasks" {
		t.Errorf("the API does not say the index is being built: %v", got.Header())
	}
	building = false
	if body := here.ask(t, "GET", "/usage", nil).Body.String(); strings.Contains(body, "still being built") {
		t.Error("a finished index still says it is being built")
	}
	empty, _ := withUsage(t)
	empty.server.SetUsageStatus(func() usage.Status { return usage.Status{Building: true, Done: 0, Total: 4} })
	if body := empty.ask(t, "GET", "/usage", nil).Body.String(); !strings.Contains(body, "0 of 4 tasks") || strings.Contains(body, "No model call has been recorded yet") {
		t.Error("an empty index that is being built is not an empty month")
	}
}

func TestTheSpendingPageNamesItsTwoPanelsByAgentAndByAgentType(t *testing.T) {
	here := open(t)
	here.signIn(t)
	body := here.ask(t, "GET", "/spend", nil).Body.String()
	if strings.Count(body, "<h2>By agent</h2>") != 1 || strings.Count(body, "<h2>By agent type</h2>") != 1 {
		t.Errorf("the panels are not by agent and by agent type:\n%s", body)
	}
}

func TestUsageIsLinkedFromTheMenuNextToSpending(t *testing.T) {
	here := open(t)
	here.signIn(t)
	body := here.ask(t, "GET", "/spend", nil).Body.String()
	if !strings.Contains(body, `href="/spend"`) || !strings.Contains(body, `href="/usage"`) {
		t.Error("the menu does not link Usage next to Spending")
	}
}

func jsonList(t *testing.T, got *httptest.ResponseRecorder, key string) []any {
	t.Helper()
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	var said map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &said); err != nil {
		t.Fatalf("%q: %v", got.Body.String(), err)
	}
	list, _ := said[key].([]any)
	return list
}

func TestTheAPIAnswersInTheShapesOfTheUsageTypes(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	secret := here.secrets["finance"]
	if sessions := jsonList(t, here.call("GET", "/api/usage/sessions?limit=1", secret), "sessions"); len(sessions) != 1 {
		t.Errorf("limit=1 gave %d sessions", len(sessions))
	}
	var page struct {
		Sessions []usage.Session `json:"sessions"`
		Next     string          `json:"next"`
	}
	got := here.call("GET", "/api/usage/sessions?limit=1", secret)
	if err := json.Unmarshal(got.Body.Bytes(), &page); err != nil || page.Next == "" || page.Sessions[0].Calls == 0 {
		t.Fatalf("the sessions came back as %s (%v)", got.Body, err)
	}
	if more := jsonList(t, here.call("GET", "/api/usage/sessions?limit=1&after="+url.QueryEscape(page.Next), secret), "sessions"); len(more) != 1 {
		t.Errorf("the next page held %d sessions", len(more))
	}
	if calls := jsonList(t, here.call("GET", "/api/usage/calls?session=alice-session", secret), "calls"); len(calls) != 2 {
		t.Errorf("alice's session held %d calls", len(calls))
	}
	if got := here.call("GET", "/api/usage/calls", secret); got.Code != http.StatusBadRequest {
		t.Errorf("calls with no session answered %d", got.Code)
	}
	var totals usage.Totals
	if err := json.Unmarshal(here.call("GET", "/api/usage/totals?person="+url.QueryEscape(alice), secret).Body.Bytes(), &totals); err != nil || totals.Calls != 2 || totals.USD != 0.5 {
		t.Errorf("the totals were %+v (%v)", totals, err)
	}
	var facets usage.Facets
	if err := json.Unmarshal(here.call("GET", "/api/usage/facets", secret).Body.Bytes(), &facets); err != nil || len(facets.People) != 2 || len(facets.Models) != 1 {
		t.Errorf("the facets were %+v (%v)", facets, err)
	}
	none, _ := withUsage(t)
	if body := none.call("GET", "/api/usage/facets", none.secrets["finance"]).Body.String(); strings.Contains(body, "null") {
		t.Errorf("an empty list should be [] and not null: %s", body)
	}
	if body := none.call("GET", "/api/usage/sessions", none.secrets["finance"]).Body.String(); !strings.Contains(body, `"sessions":[]`) {
		t.Errorf("no sessions should be [] and not null: %s", body)
	}
}

func TestOnlyATokenThatMayReadSpendingOrActsAsAPersonReadsUsage(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	for _, path := range []string{"/api/usage/sessions", "/api/usage/totals", "/api/usage/facets", "/api/usage/calls?session=alice-session", "/api/usage.csv"} {
		if got := here.call("GET", path, ""); got.Code != http.StatusUnauthorized {
			t.Errorf("%s with no token answered %d", path, got.Code)
		}
		if got := here.call("GET", path, here.secrets["ci"]); got.Code != http.StatusForbidden {
			t.Errorf("%s with a token that may only submit answered %d", path, got.Code)
		}
		if got := here.call("GET", path, here.secrets["finance"]); got.Code != http.StatusOK {
			t.Errorf("%s with a spending token answered %d", path, got.Code)
		}
		if got := here.as(t, alice).ask(t, "GET", path, nil); got.Code != http.StatusOK {
			t.Errorf("%s for a person signed in answered %d", path, got.Code)
		}
	}
}
