package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/usage"
)

// mountUsage makes the pages of the usage index, in the app and in the console, and the calls of the API that read it.
func (s *Server) mountUsage(mux *http.ServeMux) {
	mux.HandleFunc("GET /usage", s.guard(s.usagePage("/usage", false)))
	mux.HandleFunc("GET /usage/{session}", s.guard(s.usageSessionPage("/usage", false)))
	mux.HandleFunc("GET /admin/usage", s.console(s.usagePage("/admin/usage", true)))
	mux.HandleFunc("GET /admin/usage/{session}", s.console(s.usageSessionPage("/admin/usage", true)))
}

// drawUsage draws a page of the usage in the app or, for an admin, in the console.
func (s *Server) drawUsage(w http.ResponseWriter, name string, console bool, page map[string]any) {
	if console {
		s.drawConsole(w, name, "usage", page)
		return
	}
	page["Nav"] = "usage"
	s.render(w, name, page)
}

// usageView is the sessions page: the figures above the table, the table, and the controls that narrow it.
type usageView struct {
	Query        usageQuery
	Values       url.Values
	Everyone     bool
	Facets       usage.Facets
	Building     usage.Status
	Figures      usageFigures
	Rows         []usageRow
	Columns      []usageColumn
	Filtered     bool
	Newest, Next string
	Export       string
}

type usageFigures struct {
	Spent, SpentCaption, In, Out, Cache string
	Calls, Refused                      int64
}

type usageColumn struct {
	Label, Href, Sorted string
	Num                 bool
}

// usageRow is a session as a line of the table.
type usageRow struct {
	ID, Href, Record, Last, LastFull string
	Person, Agent, Engine, Harness   string
	Providers, Models                string
	Cost, Unpriced, In, Out, Cache   string
	Calls, Refused                   int64
	MoreTasks                        int
}

func (s *Server) usagePage(base string, console bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.showUsage(w, r, base, console) }
}

func (s *Server) showUsage(w http.ResponseWriter, r *http.Request, base string, console bool) {
	page := map[string]any{"Title": productName + " — usage", "Base": base}
	if s.usage.store == nil {
		s.drawUsage(w, "usage.html", console, page)
		return
	}
	query, allowed, err := s.readUsageQuery(r)
	switch {
	case !allowed:
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	view, err := s.viewUsage(r, query, base)
	if err != nil {
		http.Error(w, "the usage could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	page["Usage"] = view
	s.drawUsage(w, "usage.html", console, page)
}

func (s *Server) viewUsage(r *http.Request, query usageQuery, base string) (usageView, error) {
	ctx := r.Context()
	sessions, next, err := s.usage.store.Sessions(ctx, query.Filter, query.Page)
	if err != nil {
		return usageView{}, err
	}
	totals, err := s.usage.store.Totals(ctx, query.Filter)
	if err != nil {
		return usageView{}, err
	}
	person, _ := s.usageScope(r)
	facets, err := s.usageFacets(r, usage.Filter{From: query.Filter.From, To: query.Filter.To, Person: person})
	if err != nil {
		return usageView{}, err
	}
	facets = chosenIn(facets, query.Values)
	view := usageView{Query: query, Values: query.Values, Everyone: person == "", Facets: facets, Building: s.usage.building(),
		Figures: figuresOf(totals), Rows: rowsOf(sessions, base), Columns: columnsOf(query, base),
		Export: query.link("/api/usage.csv", map[string]string{"sort": "", "dir": "", "limit": "", "after": ""})}
	for i := range view.Rows {
		view.Rows[i].Harness = s.harnessOf(view.Rows[i].Engine)
	}
	for _, name := range usageFields {
		if name != "person" || person == "" {
			view.Filtered = view.Filtered || query.Values.Get(name) != ""
		}
	}
	if query.Page.After != "" {
		view.Newest = query.link(base, map[string]string{"after": ""})
	}
	if next != "" {
		view.Next = query.link(base, map[string]string{"after": string(next)})
	}
	return view, nil
}

func figuresOf(t usage.Totals) usageFigures {
	spent, caption := usageSpent(t)
	return usageFigures{Spent: spent, SpentCaption: caption, Calls: t.Calls, Refused: t.Refused, In: compact(t.InputTokens),
		Out: compact(t.OutputTokens), Cache: cacheRate(t.CachedTokens, t.InputTokens)}
}

// columnsOf are the headings of the table. A heading that can be sorted by is a link that sets the sort and keeps the
// filter, and the one that is sorted by is marked. The index sorts across all pages, from the greatest, and "last" is its default.
func columnsOf(query usageQuery, base string) []usageColumn {
	sortable := func(label, key string) usageColumn {
		sorted := query.Page.Sort == key || (key == "last" && query.Page.Sort == "")
		column := usageColumn{Label: label, Num: key != "last", Href: query.link(base, map[string]string{"after": "", "sort": key})}
		if sorted {
			column.Sorted = "descending"
		}
		return column
	}
	return []usageColumn{sortable("Last request (UTC)", "last"), {Label: "Session"}, {Label: "Who"}, {Label: "Agent"},
		sortable("Cost", "cost"), {Label: "Provider"}, {Label: "Model"}, sortable("Tokens", "tokens"), {Label: "Cache", Num: true}, sortable("Calls", "calls"), {Label: "Audit trail"}}
}

func rowsOf(sessions []usage.Session, base string) []usageRow {
	rows := make([]usageRow, 0, len(sessions))
	for _, each := range sessions {
		row := usageRow{ID: each.ID, Href: base + "/" + url.PathEscape(each.ID), Last: each.Last.UTC().Format("2 Jan 15:04"),
			LastFull: each.Last.UTC().Format("2006-01-02 15:04:05 UTC"), Person: each.Person, Agent: each.Agent, Engine: each.Engine,
			Providers: strings.Join(each.Providers, ", "), Models: strings.Join(each.Models, ", "), Calls: each.Calls, Refused: each.Refused,
			In: compact(each.InputTokens), Out: compact(each.OutputTokens), Cache: cacheRate(each.CachedTokens, each.InputTokens)}
		row.Cost, row.Unpriced = costOf(each.Totals)
		if n := len(each.Tasks); n > 0 {
			row.Record, row.MoreTasks = "/jobs/"+url.PathEscape(each.Tasks[n-1]), n-1
		}
		rows = append(rows, row)
	}
	return rows
}

// costOf is a session's cost as a cell reads it, and what is missing from it. A call with no price is never a call
// that cost nothing.
func costOf(t usage.Totals) (cost, unpriced string) {
	switch {
	case t.Unmeasured == 0:
		return headlineUSD(t.USD), ""
	case t.Unmeasured == t.Calls:
		return "no price", ""
	}
	return headlineUSD(t.USD), fmt.Sprintf("+ %d with no price", t.Unmeasured)
}

// usageFacets are the values the selects offer: those of the person's own calls, in the dates asked for.
func (s *Server) usageFacets(r *http.Request, filter usage.Filter) (usage.Facets, error) {
	facets, err := s.usage.store.Facets(r.Context(), filter)
	if err != nil {
		return facets, err
	}
	if filter.Person != "" {
		facets.People = []string{filter.Person}
	}
	return facets, nil
}

// callRow is one call as a line of a session's table.
type callRow struct {
	At, AtFull, Task, Record, Model, Provider, Key string
	In, Out, Cache, Cost, Refused, Translated      string
	Status                                         int
	Unpriced                                       bool
}

type sessionView struct {
	ID, Person, Agent, Engine, Search string
	Calls                             []callRow
	Next, Newest                      string
}

func (s *Server) usageSessionPage(base string, console bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.showSession(w, r, base, console) }
}

func (s *Server) showSession(w http.ResponseWriter, r *http.Request, base string, console bool) {
	if s.usage.store == nil {
		http.NotFound(w, r)
		return
	}
	query, allowed, err := s.readUsageQuery(r)
	switch {
	case !allowed:
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("session")
	rows, next, err := s.sessionCalls(r, id, query)
	switch {
	case errors.Is(err, usage.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, "the usage could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	view := sessionView{ID: id, Calls: callRows(rows), Search: query.Values.Get("q")}
	if len(rows) > 0 {
		view.Person, view.Agent, view.Engine = rows[0].Person, rows[0].Agent, rows[0].Engine
	}
	here := base + "/" + url.PathEscape(id)
	if query.Page.After != "" {
		view.Newest = query.link(here, map[string]string{"after": ""})
	}
	if next != "" {
		view.Next = query.link(here, map[string]string{"after": string(next)})
	}
	s.drawUsage(w, "usage-session.html", console, map[string]any{"Title": productName + " — usage of " + id, "Base": base, "Session": view})
}

// sessionCalls is a session's calls that match the filter, which for a person is also theirs alone. A session
// with none of the person's calls is answered as one the index does not hold, so that a guessed id says nothing
// about someone else's. A session of theirs that the filter leaves nothing of is an empty page.
func (s *Server) sessionCalls(r *http.Request, id string, query usageQuery) ([]usage.Row, usage.Cursor, error) {
	store := s.usage.store
	rows, next, err := store.Calls(r.Context(), id, query.Filter, query.Page)
	if err != nil {
		return nil, "", err
	}
	person, _ := s.usageScope(r)
	kept := rows[:0:0]
	for _, row := range rows {
		if person == "" || strings.EqualFold(row.Person, person) {
			kept = append(kept, row)
		}
	}
	if len(kept) > 0 || person == "" {
		return kept, next, nil
	}
	own, _, err := store.Calls(r.Context(), id, usage.Filter{Person: person}, usage.Page{Limit: 1})
	if err != nil {
		return nil, "", err
	}
	if len(own) == 0 {
		return nil, "", usage.ErrNotFound
	}
	return kept, "", nil
}

func callRows(rows []usage.Row) []callRow {
	out := make([]callRow, 0, len(rows))
	for _, row := range rows {
		call := callRow{At: row.At.UTC().Format("2 Jan 15:04:05"), AtFull: row.At.UTC().Format("2006-01-02 15:04:05 UTC"), Task: row.Task,
			Record: "/jobs/" + url.PathEscape(row.Task), Model: row.Model, Provider: row.Provider, Key: row.Credential, In: tally(row.InputTokens),
			Out: tally(row.OutputTokens), Cache: "—", Cost: "no price", Status: row.Status, Refused: row.Refused, Translated: row.Translated,
			Unpriced: row.USD == nil}
		if row.USD != nil {
			call.Cost = headlineUSD(*row.USD)
		}
		if row.InputTokens != nil && row.CachedTokens != nil {
			call.Cache = cacheRate(*row.CachedTokens, *row.InputTokens)
		}
		out = append(out, call)
	}
	return out
}

// chosenIn keeps a value the person chose in the list of its select, so that a choice the dates have since left out is
// still shown as chosen.
func chosenIn(f usage.Facets, chosen url.Values) usage.Facets {
	for name, list := range map[string]*[]string{"person": &f.People, "agent": &f.Agents, "agent_type": &f.Engines, "provider": &f.Providers, "model": &f.Models} {
		if value := chosen.Get(name); value != "" && !slices.Contains(*list, value) {
			*list = append(*list, value)
			slices.Sort(*list)
		}
	}
	return f
}
