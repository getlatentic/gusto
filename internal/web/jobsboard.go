package web

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/spend"
)

// perPage is how many jobs one page of the record shows. The record only
// grows, and a page that draws every job ever run gets slower every day.
const perPage = 50

// figure is one of the counts across the top of the record, and the filter it
// stands for.
type figure struct {
	Label   string
	Count   int
	Href    string
	Current bool
}

// board is the record as a dashboard: what needs a person now, then every
// job the filters let through, a page at a time.
type board struct {
	Figures   []figure
	State     string
	Harness   string
	Harnesses []string
	Runs      []run
	Shown     string
	Newer     string
	Older     string
}

// lately is how far back the counts across the top look. What is running or
// queued is counted whenever it started, because it is still happening.
const lately = 7 * 24 * time.Hour

// filed is which of the filters a job falls under. An interrupted job did not
// pass, so it is found with the failures.
func filed(state jobs.State) string {
	if state == jobs.Interrupted {
		return string(jobs.Failed)
	}
	return string(state)
}

func (s *Server) board(r *http.Request) board {
	query := r.URL.Query()
	shown := board{State: query.Get("state"), Harness: query.Get("harness")}
	all := s.index.List()
	since := time.Now().Add(-lately).Unix()
	counts := map[string]int{}
	var kept []jobs.Record
	for _, record := range all {
		if !record.State.Done() || record.Queued >= since {
			counts[filed(record.State)]++
		}
		if !slices.Contains(shown.Harnesses, record.Harness) {
			shown.Harnesses = append(shown.Harnesses, record.Harness)
		}
		if (shown.State == "" || filed(record.State) == shown.State) && (shown.Harness == "" || record.Harness == shown.Harness) {
			kept = append(kept, record)
		}
	}
	slices.Sort(shown.Harnesses)
	for _, state := range []jobs.State{jobs.Running, jobs.Queued, jobs.Passed, jobs.Failed} {
		shown.Figures = append(shown.Figures, figure{
			Label: string(state), Count: counts[string(state)],
			Href:    filtered(query, "state", string(state)),
			Current: shown.State == string(state),
		})
	}
	s.page(&shown, kept, query)
	return shown
}

// page cuts one page out of the jobs the filters kept, and says where the
// pages either side of it are.
func (s *Server) page(shown *board, kept []jobs.Record, query url.Values) {
	at, _ := strconv.Atoi(query.Get("page"))
	at = max(at, 0)
	from := min(at*perPage, len(kept))
	to := min(from+perPage, len(kept))
	ledger, _ := s.spending()
	for _, record := range kept[from:to] {
		shown.Runs = append(shown.Runs, s.recorded(record, ledger))
	}
	if len(kept) > 0 {
		shown.Shown = strconv.Itoa(from+1) + "–" + strconv.Itoa(to) + " of " + strconv.Itoa(len(kept))
	}
	if at > 0 {
		shown.Newer = filtered(query, "page", strconv.Itoa(at-1))
	}
	if to < len(kept) {
		shown.Older = filtered(query, "page", strconv.Itoa(at+1))
	}
}

// filtered is the page's own address with one filter changed. Asking for the
// filter already on turns it off, and any change but paging starts again at
// the first page.
func filtered(query url.Values, key, value string) string {
	next := url.Values{}
	for name, values := range query {
		next[name] = slices.Clone(values)
	}
	if key != "page" {
		next.Del("page")
	}
	if next.Get(key) == value && key != "page" {
		next.Del(key)
	} else {
		next.Set(key, value)
	}
	if len(next) == 0 {
		return "/jobs"
	}
	return "/jobs?" + next.Encode()
}

// spentBy is a job's spending as the proxy counted it, when the ledger has
// any: the provider's own count, rather than the harness's claim about itself.
func spentBy(ledger *spend.Ledger, id string) (spend.Tally, bool) {
	if ledger == nil {
		return spend.Tally{}, false
	}
	return ledger.Job(id)
}
