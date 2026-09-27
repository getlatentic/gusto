package web

import (
	"net/http"
	"sort"

	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
)

// yours is an instance as the home page lists it: what it is, what was last
// said to it, and where to pick that conversation up.
type yours struct {
	instance.Instance
	Threads int
	Where   string
	Latest  jobs.Record
	// Active is when it was last spoken to, or started when it never was.
	Active int64
}

// home is the harnesses somebody already has, the one used last first,
// because a person coming back wants to carry on rather than choose again.
// Choosing is the catalog's page.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	held := s.instances.List()
	listed := make([]yours, 0, len(held))
	for _, each := range held {
		listed = append(listed, s.lately(each))
	}
	sort.SliceStable(listed, func(a, b int) bool { return listed[a].Active > listed[b].Active })
	s.render(w, "home.html", map[string]any{
		"Title":     "haas",
		"Nav":       "harnesses",
		"Instances": listed,
		"Refused":   r.URL.Query().Get("refused"),
	})
}

// catalogPage is every harness this deployment carries, and whether it can
// start each one.
func (s *Server) catalogPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "catalog.html", map[string]any{
		"Title":   "haas — start a harness",
		"Nav":     "new",
		"Catalog": s.catalog,
		"Refused": r.URL.Query().Get("refused"),
	})
}

// lately is where to pick an instance up and what it was last doing. Every
// instance has a conversation to go back to, because the store opens one.
func (s *Server) lately(held instance.Instance) yours {
	last := s.instances.Latest(held.ID)
	latest := s.latestTurn(last)
	active := held.Started
	if latest.Queued > active {
		active = latest.Queued
	}
	return yours{
		Instance: held,
		Threads:  len(s.instances.Threads(held.ID)),
		Where:    threadPath(last),
		Latest:   latest,
		Active:   active,
	}
}

// removeInstance takes an instance off the list. One with a turn still queued
// or running keeps it, because that turn is working in its folder.
func (s *Server) removeInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, found := s.instances.Get(id); !found {
		http.NotFound(w, r)
		return
	}
	for _, record := range s.index.List() {
		if record.Instance == id && !record.State.Done() {
			s.turnAway(w, r, "/instances/"+id, "a turn is still "+string(record.State)+" here; remove it once that ends")
			return
		}
	}
	if err := s.instances.Remove(id); err != nil {
		s.turnAway(w, r, "/instances/"+id, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
