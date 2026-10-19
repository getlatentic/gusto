package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/taskindex"
)

// badQuery is a request the list page cannot read, which is the asker's mistake and not the server's.
type badQuery struct{ error }

// sortChoices are the orders the list offers, by the name the query gives.
var sortChoices = []struct{ Name, Label string }{{"queued", "Newest asked"}, {"ended", "Recently ended"}, {"seq", "Order of arrival"}}

// indexed is what the list page adds to the board when it reads the index: the search box, the choices that narrow
// the list further, the order, and how far the index has caught up.
type indexed struct {
	Query    string
	Person   string
	Everyone bool
	Sort     string
	Sorts    []struct{ Name, Label string }
	Building taskindex.Status
	Newest   string
}

// boardFor is the record the request asks for, from the index when there is one and from memory otherwise.
func (s *Server) boardFor(r *http.Request) (board, error) {
	if s.listing.store == nil {
		return s.board(r), nil
	}
	return s.boardFromIndex(r)
}

func (s *Server) boardFromIndex(r *http.Request) (board, error) {
	query, err := s.readTaskQuery(r, true)
	if err != nil {
		return board{}, badQuery{err}
	}
	person, _ := s.taskScope(r)
	shown := board{State: query.State, Harness: query.Values.Get("agent_type"), Index: &indexed{
		Query: query.Values.Get("q"), Person: query.Values.Get("person"), Everyone: person == "", Sort: query.Page.Sort,
		Sorts: sortChoices, Building: s.listing.building()}}
	if query.None {
		return shown, nil
	}
	if err := s.fillBoard(r, &shown, query); err != nil {
		return board{}, err
	}
	return shown, nil
}

func (s *Server) fillBoard(r *http.Request, shown *board, query taskQuery) error {
	ctx, store := r.Context(), s.listing.store
	scope := taskindex.Filter{Asked: query.Filter.Asked}
	counts, err := s.figuresOf(r, scope)
	if err != nil {
		return err
	}
	for _, state := range []jobs.State{jobs.Running, jobs.Queued, jobs.Passed, jobs.Failed} {
		shown.Figures = append(shown.Figures, figure{
			Label: string(state), Count: counts[string(state)],
			Href:    filtered(r.URL.Query(), "state", string(state)),
			Current: shown.State == string(state),
		})
	}
	types, err := store.AgentTypes(ctx, scope)
	if err != nil {
		return err
	}
	shown.Harnesses = slices.DeleteFunc(types, func(name string) bool { return name == "" })
	rows, next, err := store.List(ctx, query.Filter, query.Page)
	if errors.Is(err, taskindex.ErrBadCursor) {
		return badQuery{err}
	}
	if err != nil {
		return err
	}
	matching, err := store.Count(ctx, query.Filter)
	if err != nil {
		return err
	}
	ledger, _ := s.spending()
	for _, row := range rows {
		shown.Runs = append(shown.Runs, s.recorded(jobs.RecordOf(row), ledger))
	}
	if total := totalOf(matching); total > 0 {
		shown.Shown = strconv.Itoa(total) + " " + plural(total, "task")
	}
	s.pageLinks(r, shown, query, next)
	return nil
}

// figuresOf counts what the figures across the top show: what is running or queued whenever it started, and the
// tasks that ended lately. Interrupted tasks are counted with the failures.
func (s *Server) figuresOf(r *http.Request, scope taskindex.Filter) (map[string]int, error) {
	active, done := scope, scope
	active.States = []string{string(jobs.Running), string(jobs.Queued)}
	done.States = []string{string(jobs.Passed), string(jobs.Failed), string(jobs.Interrupted)}
	done.QueuedFrom = sinceLately(time.Now())
	counts := map[string]int{}
	for _, f := range []taskindex.Filter{active, done} {
		got, err := s.listing.store.Count(r.Context(), f)
		if err != nil {
			return nil, err
		}
		for state, n := range got {
			counts[filed(jobs.State(state))] += n
		}
	}
	return counts, nil
}

// pageLinks day-groups a page that is read in the order tasks were asked, and links to the page after it and
// back to the start. A listing in another order has no days to speak of.
func (s *Server) pageLinks(r *http.Request, shown *board, query taskQuery, next taskindex.Cursor) {
	if query.Page.Sort == "" || query.Page.Sort == string(taskindex.ByQueued) {
		shown.Days = byDay(shown.Runs, time.Now())
	} else if len(shown.Runs) > 0 {
		shown.Days = []day{{Runs: shown.Runs}}
	}
	if query.Page.After != "" {
		shown.Index.Newest = filtered(r.URL.Query(), "after", "")
	}
	if next != "" {
		shown.Older = filtered(r.URL.Query(), "after", string(next))
	}
}

func totalOf(counts map[string]int) int {
	total := 0
	for _, n := range counts {
		total += n
	}
	return total
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
