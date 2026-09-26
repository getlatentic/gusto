package web

import (
	"net/http"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/jobs"
)

// sentOverHTTP is who asked for a job that arrived at POST /api/jobs. The
// server knows how such a job arrived and not who sent it.
const sentOverHTTP = "sent over HTTP"

// run is one line of the record: the job, what it cost, and what it ran on.
type run struct {
	jobs.Record
	Cost *float64
	On   origin
}

// origin is where a run came from. Link is empty when there is nothing to go
// to, and Named then says why, so the record never shows an empty link.
type origin struct {
	Named string
	Link  string
}

// list is the record: every run this server knows, whatever started it, with
// its verdict, its cost and the way to its evidence. It starts nothing itself;
// a run happens on an instance, and the page says where to start one.
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	records := s.index.List()
	runs := make([]run, 0, len(records))
	for _, record := range records {
		runs = append(runs, s.recorded(record))
	}
	s.render(w, "list.html", map[string]any{
		"Title":     "haas",
		"Nav":       "jobs",
		"Subtitle":  "every run, and what it left behind",
		"Runs":      runs,
		"Harnesses": catalog.Ready(s.catalog),
	})
}

// recorded is a job as the record shows it. The cost is the manifest's,
// because the manifest is what the run signed; a run that left none reported
// no cost, which the page shows as exactly that.
func (s *Server) recorded(record jobs.Record) run {
	shown := run{Record: record, On: s.origin(record)}
	if manifest := readManifest(record.Dir(s.index.Root())); manifest != nil {
		shown.Cost = manifest.CostUSD
	}
	return shown
}

// origin names the instance a run belonged to and links to it. A job written
// before runs recorded their instance, or sent from somewhere else, has none,
// and it still lists and opens.
func (s *Server) origin(record jobs.Record) origin {
	if record.Instance == "" {
		return origin{Named: withoutInstance(record.Asked)}
	}
	held, found := s.instances.Get(record.Instance)
	if !found {
		return origin{Named: record.Instance + ", which this server no longer holds"}
	}
	return origin{Named: held.Harness + " · " + held.ID, Link: "/instances/" + held.ID}
}

// withoutInstance says how a run with no instance arrived, as far as the
// server can tell from who asked for it.
func withoutInstance(asked string) string {
	if asked == sentOverHTTP {
		return "none: sent to POST /api/jobs, from the command line or another machine"
	}
	return "none: made before runs belonged to an instance"
}
