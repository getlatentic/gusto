package web

import (
	"net/http"
	"strconv"
)

// perTable is how many rows one page of a console table shows.
const perTable = 25

// pager says which rows of a table one page shows, and where the pages either side of it are. A table that fits one page has none.
type pager struct {
	Shown, Previous, Next string
}

// onePage cuts the page the request names, counted from 1, out of rows. here is the page's own address with its query, which
// the links to the other pages add to.
func onePage[T any](r *http.Request, here string, rows []T) ([]T, pager) {
	at, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || at < 1 {
		at = 1
	}
	last := max((len(rows)+perTable-1)/perTable, 1)
	at = min(at, last)
	from, to := (at-1)*perTable, min(at*perTable, len(rows))
	if last == 1 {
		return rows, pager{}
	}
	shown := pager{Shown: strconv.Itoa(from+1) + "–" + strconv.Itoa(to) + " of " + strconv.Itoa(len(rows))}
	if at > 1 {
		shown.Previous = pageLink(here, at-1)
	}
	if at < last {
		shown.Next = pageLink(here, at+1)
	}
	return rows[from:to], shown
}

func pageLink(here string, at int) string {
	if at == 1 {
		return here
	}
	return here + "&page=" + strconv.Itoa(at)
}
