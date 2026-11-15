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

// usageView is the sessions page: the figures above the table, the table, and the controls that narrow it.
type usageView struct {
	Query    usageQuery
	Values   url.Values
	Everyone bool
	Facets   usage.Facets
	Building usage.Status
	Figures  usageFigures
	Rows     []usageRow
	Columns  []usageColumn
	States   []usageState
	// ShowWho is whether the table is everyone's or holds more than one asker, so a column naming them says something.
	ShowWho  bool
	Filtered bool
	// Clear is the page with no filter, on the same view.
	Clear        string
	Newest, Next string
	Export       string
}

type usageFigures struct {
	Spent, SpentCaption, In, Out, Cache string
	Unpriced                            string
	Calls, Refused                      int64
}

type usageColumn struct {
	Label, Href, Sorted string
	Num                 bool
}

// usageRow is a session as a line of the table.
type usageRow struct {
	ID, Title, Href, Record, Last, LastFull string
	Person, Agent, Engine, Harness          string
	// Mark is the agent type's logo, and MarkDark the one for a dark page when its owner publishes one.
	Mark, MarkDark string
	// Asked is who asked for the conversation's latest turn, and Owner whose agent it is, which is who pays for its calls.
	Asked, Owner                   string
	Providers, Models              string
	Cost, Unpriced, In, Out, Cache string
	Calls, Refused                 int64
	// Turns is how many tasks the conversation's calls came from, so a row that adds up several reads as a total.
	Turns int
}

// usageData is the sessions page: a person's own at /usage, and everyone's in the console.
func (s *Server) usageData(r *http.Request, base string, console bool) (map[string]any, error) {
	page := map[string]any{"Base": base, "Tabs": tabsFor(console, "tasks")}
	// A person sees their own month against their budget before they reach it.
	if mine := s.memberBudgetOf(s.askerName(r)); !console && mine.Set {
		page["Mine"] = mine
	}
	if s.usage.store == nil {
		return page, nil
	}
	query, allowed, err := s.readUsageQuery(r)
	switch {
	case !allowed:
		return nil, failed{http.StatusNotFound, "not found"}
	case err != nil:
		return nil, failed{http.StatusBadRequest, err.Error()}
	}
	view, err := s.viewUsage(r, query, base)
	if err != nil {
		return nil, failed{http.StatusInternalServerError, "the usage could not be read: " + err.Error()}
	}
	page["Usage"] = view
	return page, nil
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
		Figures: figuresOf(totals, query.Values), Rows: rowsOf(sessions, base), Columns: columnsOf(query, base),
		Export: query.link("/api/usage.csv", map[string]string{"sort": "", "dir": "", "limit": "", "after": ""}),
		States: s.usageStates(query, base), Filtered: filteredBeyond(query.Values, person), Clear: clearedOf(query.Values, base)}
	if view.ShowWho = s.describeRows(view.Rows) > 1 || view.Everyone; !view.ShowWho {
		view.Columns = slices.DeleteFunc(view.Columns, func(column usageColumn) bool { return column.Label == askedBy })
	}
	if query.Page.After != "" {
		view.Newest = query.link(base, map[string]string{"after": ""})
	}
	if next != "" {
		view.Next = query.link(base, map[string]string{"after": string(next)})
	}
	return view, nil
}

func figuresOf(t usage.Totals, values url.Values) usageFigures {
	spent, caption := usageSpent(t, usagePeriod(values))
	return usageFigures{Spent: spent, SpentCaption: caption, Unpriced: unpricedNote(t), Calls: t.Calls, Refused: t.Refused, In: compact(t.InputTokens),
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
	return []usageColumn{sortable("Last request", "last"), {Label: "Conversation"}, {Label: askedBy}, {Label: "Agent · owner"},
		sortable("Cost", "cost"), {Label: "Model"}}
}

func rowsOf(sessions []usage.Session, base string) []usageRow {
	rows := make([]usageRow, 0, len(sessions))
	for _, each := range sessions {
		row := usageRow{ID: each.ID, Href: base + "/" + url.PathEscape(each.ID), Last: each.Last.Local().Format("2 Jan 15:04"),
			LastFull: each.Last.Local().Format("2006-01-02 15:04:05 MST"), Person: each.Person, Agent: each.Agent, Engine: each.Engine,
			Providers: strings.Join(each.Providers, ", "), Models: strings.Join(each.Models, ", "), Calls: each.Calls, Refused: each.Refused,
			In: compact(each.InputTokens), Out: compact(each.OutputTokens), Cache: cacheRate(each.CachedTokens, each.InputTokens)}
		row.Cost, row.Unpriced = costOf(each.Totals)
		if n := len(each.Tasks); n > 0 {
			row.Record, row.Turns = "/jobs/"+url.PathEscape(each.Tasks[n-1]), n
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
	return headlineUSD(t.USD), fmt.Sprintf("+ %d %s with no price", t.Unmeasured, plural(int(t.Unmeasured), "call"))
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

// sessionData is one session's calls, under the usage page at base.
func (s *Server) sessionData(r *http.Request, base string) (map[string]any, error) {
	if s.usage.store == nil {
		return nil, failed{http.StatusNotFound, "not found"}
	}
	query, allowed, err := s.readUsageQuery(r)
	switch {
	case !allowed:
		return nil, failed{http.StatusNotFound, "not found"}
	case err != nil:
		return nil, failed{http.StatusBadRequest, err.Error()}
	}
	id := r.PathValue("session")
	rows, next, err := s.sessionCalls(r, id, query)
	switch {
	case errors.Is(err, usage.ErrNotFound):
		return nil, failed{http.StatusNotFound, "not found"}
	case err != nil:
		return nil, failed{http.StatusInternalServerError, "the usage could not be read: " + err.Error()}
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
	return map[string]any{"Base": base, "Session": view}, nil
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
