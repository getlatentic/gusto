package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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
	if page := here.ask(t, "GET", "/usage", nil); page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<div id="root">`) {
		t.Fatalf("the usage page is not the app: %d", page.Code)
	}
	view := usageAt(t, here, "/usage").Usage
	if got := view.Figures; got.Spent != "$0.7500" || got.Calls != 3 || got.In != "3.0k" || got.Cache != "40%" {
		t.Errorf("the figures do not add everyone's calls up: %+v", got)
	}
	if !view.Everyone || !view.ShowWho || !slices.Equal(view.Facets.People, []string{alice, bob}) {
		t.Errorf("an admin's own usage is not everyone's: %+v", view)
	}
	asked := map[string]string{}
	for _, row := range view.Rows {
		asked[row.ID] = row.Asked
		if row.Agent != "Notes" || row.Engine != "Claude Code" || row.Providers != "bedrock" || row.Models != "claude-sonnet" {
			t.Errorf("a row does not say which agent and model: %+v", row)
		}
		if row.Href != "/usage/"+row.ID {
			t.Errorf("a row does not open its session under /usage: %q", row.Href)
		}
	}
	if asked["alice-session"] == "" || asked["bob-session"] == "" || asked["alice-session"] == asked["bob-session"] {
		t.Errorf("the rows do not say who asked: %v", asked)
	}
	if calls := sessionAt(t, here, "/usage/alice-session").Session.Calls; len(calls) == 0 || calls[0].Record != "/jobs/task-a" {
		t.Errorf("the session page does not link to its task: %+v", calls)
	}
}

// A conversation's row adds up every turn in it, and says how many, so its cost reads as a total and not as one reply's.
func TestAConversationsRowSaysHowManyTurnsItAddsUp(t *testing.T) {
	here, _ := withUsage(t, aCall("turn-1", 0, "room", alice, 0), aCall("turn-2", 0, "room", alice, 1), aCall("only", 0, "chat", bob, 2))
	page := usageAt(t, here, "/usage")
	if page.Usage.Columns[1].Label != "Conversation" || page.Tabs.Tasks != "/usage" || page.Tabs.Current != "tasks" {
		t.Errorf("the rows are not named conversations: %+v %+v", page.Usage.Columns, page.Tabs)
	}
	turns := map[string]int{}
	for _, row := range page.Usage.Rows {
		turns[row.ID] = row.Turns
	}
	if turns["room"] != 2 || turns["chat"] != 1 {
		t.Errorf("a conversation does not say how many turns its cost adds up: %v", turns)
	}
}

func TestACallWithNoPriceIsShownAsNoPriceAndNeverAsFree(t *testing.T) {
	unpriced := aCall("task-c", 0, "free-looking", alice, 3)
	unpriced.USD = nil
	unpriced.InputTokens, unpriced.CachedTokens = nil, nil
	mixed := aCall("task-d", 0, "mixed", alice, 4)
	here, _ := withUsage(t, unpriced, mixed, aCall("task-d", 1, "mixed", alice, 5))
	if figures := usageAt(t, here, "/usage").Usage.Figures; figures.Spent == "$0.0000" {
		t.Errorf("the headline should not be a sum that looks complete: %+v", figures)
	}
	only := usageAt(t, here, "/usage?q=free-looking").Usage
	if len(only.Rows) != 1 || only.Rows[0].Cost != "no price" || only.Figures.Spent != "no price" || !strings.Contains(only.Figures.SpentCaption, "no price set for these models") {
		t.Errorf("an unpriced session was shown as a cost: %+v", only)
	}
	calls := sessionAt(t, here, "/usage/free-looking").Session.Calls
	if len(calls) != 1 || !calls[0].Unpriced || calls[0].Cost != "no price" {
		t.Errorf("an unpriced call was shown as a cost: %+v", calls)
	}
	partly := usageAt(t, here, "/usage").Usage.Figures
	if partly.Unpriced != "1 call has no price on file, so it is not in these totals." || strings.Contains(partly.SpentCaption, "spent;") {
		t.Errorf("a total that leaves calls out should say so in one line under the table, not in the headline: %+v", partly)
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
		listed := usageAt(t, here, "/usage?"+c.query).sessionsOf()
		for _, want := range c.shows {
			if !slices.Contains(listed, want) {
				t.Errorf("%s does not show %s", c.query, want)
			}
		}
		for _, hidden := range c.hides {
			if slices.Contains(listed, hidden) {
				t.Errorf("%s shows %s", c.query, hidden)
			}
		}
	}
	if got := here.ask(t, "GET", "/app/api/usage?from=yesterday", nil); got.Code != http.StatusBadRequest {
		t.Errorf("a date that is not one answered %d", got.Code)
	}
	if got := usageAt(t, here, "/usage?q=nothing-is-called-this").Usage; len(got.Rows) != 0 || !got.Filtered || got.Clear != "/usage" {
		t.Errorf("a search that finds nothing should say so, with the way to clear it: %+v", got)
	}
}

func TestTheSelectsOfTheFilterListWhatTheCallsHold(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	view := usageAt(t, here, "/usage?agent=Notes").Usage
	facets := view.Facets
	if !view.Everyone || len(facets.People) != 2 || !slices.Equal(facets.Engines, []string{"Claude Code"}) || !slices.Equal(facets.Providers, []string{"bedrock"}) ||
		!slices.Equal(facets.Models, []string{"claude-sonnet"}) || !slices.Equal(facets.Agents, []string{"Notes"}) {
		t.Errorf("the filter does not offer what the calls hold: %+v", facets)
	}
	if !slices.Equal(view.Values["agent"], []string{"Notes"}) {
		t.Errorf("the filter does not keep what was chosen: %v", view.Values)
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
		page := usageAt(t, here, path).Usage
		for _, id := range page.sessionIDs() {
			seen[id]++
		}
		if pages > 0 && page.Newest != "/usage?limit=2" {
			t.Errorf("a later page does not lead back to the newest: %q", page.Newest)
		}
		path = page.Next
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
			page := usageAt(t, here, path).Usage
			ids = append(ids, page.sessionIDs()...)
			path = page.Next
		}
		return ids
	}
	if got := strings.Join(order("/usage?sort=cost&limit=1"), ""); got != "s-bs-ds-as-c" {
		t.Errorf("by cost across pages: %s", got)
	}
	if got := strings.Join(order("/usage?limit=3"), ""); got != "s-as-bs-cs-d" {
		t.Errorf("by last use: %s", got)
	}
	columns := map[string]string{}
	for _, column := range usageAt(t, here, "/usage?sort=cost&q=s-").Usage.Columns {
		columns[column.Label] = column.Sorted + " " + column.Href
		if column.Label == "Cost" && !column.Num {
			t.Error("the cost column is not a number's")
		}
	}
	if columns["Cost"] != "descending /usage?q=s-&sort=cost" {
		t.Errorf("the sorted column is not marked, or the filter is lost from its link: %v", columns)
	}
	if columns["Last request"] != " /usage?q=s-&sort=last" {
		t.Errorf("the other headings should set the sort and keep the filter: %v", columns)
	}
	api := jsonList(t, here.call("GET", "/api/usage/sessions?sort=calls&limit=2", here.secrets["finance"]), "sessions")
	if len(api) != 2 {
		t.Errorf("the API gave %d sessions", len(api))
	}
}

func TestAnUnknownSortIsRefusedAndNamesTheFour(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	for _, path := range []string{"/app/api/usage?sort=color", "/api/usage/sessions?sort=color"} {
		got := here.call("GET", path, here.secrets["finance"])
		if strings.HasPrefix(path, "/app/") {
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
	page := sessionAt(t, here, "/usage/alice-session?q=rare").Session
	if len(page.Calls) != 1 || page.Calls[0].Model != "rare-model" || page.Calls[0].Task != "task-a" || page.Search != "rare" {
		t.Errorf("the session page was not narrowed: %+v", page)
	}
	if page := sessionAt(t, here, "/usage/alice-session?q=zzz").Session; len(page.Calls) != 0 || page.ID != "alice-session" {
		t.Errorf("a search with no match is not an empty session: %+v", page)
	}
	if page := sessionAt(t, here.as(t, alice), "/usage/alice-session?q=zzz").Session; len(page.Calls) != 0 {
		t.Errorf("a person's own session with no match is not an empty session: %+v", page)
	}
}

func TestTheConsoleHasAUsagePageNextToSpending(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	if got := here.ask(t, "GET", "/admin/usage", nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `<div id="root">`) {
		t.Errorf("the console usage page is not the app: %d", got.Code)
	}
	page := usagePageOf(t, here, "")
	if page.Tabs.Summary != "/admin/spending" || page.Tabs.Current != "tasks" || !slices.Contains(page.sessionsOf(), "alice-session") {
		t.Errorf("the console usage is not next to spending, or leaves out alice's session: %+v", page)
	}
	var session map[string]any
	here.data(t, "/admin/usage/alice-session", &session)
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
	calls := sessionAt(t, here, "/usage/alice-session").Session.Calls
	if len(calls) != 3 || calls[0].Task != "task-z" || calls[0].Record != "/jobs/task-z" || calls[2].Task != "task-a" {
		t.Fatalf("the latest call should be first: %+v", calls)
	}
	first, refusal := calls[0], calls[1]
	if first.Key != "team-key" || first.Model != "claude-sonnet" || first.Provider != "bedrock" || first.Cache != "40%" {
		t.Errorf("a call does not say its key, model, provider and cache: %+v", first)
	}
	if refusal.Refused != "the team's daily cap" || refusal.Status != 429 || refusal.Translated != "anthropic to openai" || !refusal.Unpriced {
		t.Errorf("a refused call does not say why: %+v", refusal)
	}
	if got := here.ask(t, "GET", "/app/api/usage/nobody-made-this", nil); got.Code != http.StatusNotFound {
		t.Errorf("a session the index lacks answered %d", got.Code)
	}
}

func TestTheUsagePageSaysWhenThereIsNothingYet(t *testing.T) {
	here, _ := withUsage(t)
	if view := usageAt(t, here, "/usage").Usage; view == nil || len(view.Rows) != 0 || view.Filtered || view.Building.Building {
		t.Errorf("the empty state is not shown: %+v", view)
	}
	bare := open(t)
	bare.signIn(t)
	if page := usageAt(t, bare, "/usage"); page.Usage != nil {
		t.Errorf("a server with no index should say so: %+v", page)
	}
}

func TestAnIndexStillBeingBuiltSaysHowFarItIs(t *testing.T) {
	here, _ := withUsage(t, bothPeople()...)
	building := true
	here.server.SetUsageStatus(func() usage.Status { return usage.Status{Building: building, Done: 3, Total: 12} })
	if got := usageAt(t, here, "/usage").Usage.Building; !got.Building || got.Done != 3 || got.Total != 12 {
		t.Errorf("the page does not say the index is being built: %+v", got)
	}
	if got := here.call("GET", "/api/usage/totals", here.secrets["finance"]); got.Header().Get("Usage-Index-Building") != "3 of 12 tasks" {
		t.Errorf("the API does not say the index is being built: %v", got.Header())
	}
	building = false
	if usageAt(t, here, "/usage").Usage.Building.Building {
		t.Error("a finished index still says it is being built")
	}
	empty, _ := withUsage(t)
	empty.server.SetUsageStatus(func() usage.Status { return usage.Status{Building: true, Done: 0, Total: 4} })
	if got := usageAt(t, empty, "/usage").Usage.Building; !got.Building || got.Done != 0 || got.Total != 4 {
		t.Errorf("an empty index that is being built is not an empty month: %+v", got)
	}
}

func TestUsageAndSpendingLinkToEachOther(t *testing.T) {
	here := open(t)
	here.signIn(t)
	if tabs := usageAt(t, here, "/usage").Tabs; tabs.Summary != "/spend" || tabs.Tasks != "/usage" || tabs.Current != "tasks" {
		t.Errorf("the usage page does not link to spending: %+v", tabs)
	}
	var spend struct {
		Base string
		Tabs struct{ Summary, Tasks, Current string }
	}
	here.data(t, "/spend", &spend)
	if spend.Base != "/spend" || spend.Tabs.Tasks != "/usage" || spend.Tabs.Summary != "/spend" || spend.Tabs.Current != "summary" {
		t.Errorf("the spending page does not link to usage: %+v", spend)
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
