package web

import (
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/spend"
)

// run is one line of the record: the job, what it cost, and what it ran on.
// Spent is the proxy's count when the job reached a model through it, and Cost
// the harness's own claim when it made one.
type run struct {
	jobs.Record
	Cost    *float64
	Spent   spend.Tally
	Metered bool
	On      origin
}

// origin is where a run came from. Link is empty when there is nothing to go
// to; Named is then a short word for a list, and Why the whole reason, so the
// record never shows an empty link.
type origin struct {
	Named string
	// Project is the agent alone, where Named adds the conversation to it.
	Project string
	Why     string
	Link    string
}

// origin names the instance a run belonged to and links to it. A job written
// before runs recorded their instance, or sent from somewhere else, has none,
// and it still lists and opens.
func (s *Server) origin(record jobs.Record) origin {
	if record.Instance == "" {
		if record.Asked == sentOverHTTP || strings.HasPrefix(record.Asked, tokenAsker) {
			return origin{Named: "API", Why: "none: sent to POST /api/jobs, from the command line or another machine"}
		}
		return origin{Named: "before instances", Why: "none: made before runs belonged to an instance"}
	}
	held, found := s.instances.Get(record.Instance)
	if !found {
		return origin{Named: "removed", Why: record.Instance + ", which this server no longer holds"}
	}
	named := s.agentOf(held).Name
	shown := origin{Named: named, Project: named, Why: held.ID, Link: "/instances/" + held.ID}
	// A turn names its conversation, since a message in a thread is the thing
	// somebody went to the record looking for.
	if thread, found := s.instances.Thread(held.ID, record.Thread); found && record.Thread != "" {
		shown.Named, shown.Link = named+" · "+s.nameOf(thread), threadPath(thread)
	}
	return shown
}

// list is the record: every run this server knows, whatever started it, with
// what needs a person counted across the top. It starts nothing itself; a run
// happens on an instance.
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	s.render(w, "list.html", map[string]any{
		"Title": productName + " — tasks",
		"Nav":   "jobs",
		"Board": s.board(r),
	})
}

// recorded is a job as the record shows it.
func (s *Server) recorded(record jobs.Record, ledger *spend.Ledger) run {
	shown := run{Record: record, On: s.origin(record)}
	shown.Spent, shown.Metered = spentBy(ledger, record.ID)
	if manifest := readManifest(record.Dir(s.index.Root())); manifest != nil {
		shown.Cost = manifest.CostUSD
	}
	return shown
}
