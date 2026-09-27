package web

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/industry"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
)

// yours is an instance as the home page lists it: what it is, what was last
// said to it, and where to pick that conversation up.
type yours struct {
	agent
	Threads int
	Where   string
	Latest  jobs.Record
	// Active is when it was last spoken to, or started when it never was.
	Active int64
}

// agentsPerPage is how many rows the home page shows at once.
const agentsPerPage = 10

// home is the agents somebody already has, the one used last first,
// because a person coming back wants to carry on rather than choose again.
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	held := s.instances.List()
	listed := make([]yours, 0, len(held))
	for _, each := range held {
		listed = append(listed, s.lately(each))
	}
	sort.SliceStable(listed, func(a, b int) bool { return listed[a].Active > listed[b].Active })
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	page = max(page, 1)
	from := min((page-1)*agentsPerPage, len(listed))
	to := min(from+agentsPerPage, len(listed))
	data := map[string]any{
		"Title":     "haas",
		"Nav":       "harnesses",
		"Instances": listed[from:to],
		"Total":     len(listed),
		"From":      from + 1,
		"To":        to,
		"Templates": industry.All(),
		"Refused":   r.URL.Query().Get("refused"),
	}
	if page > 1 {
		data["Newer"] = "/?page=" + strconv.Itoa(page-1)
	}
	if to < len(listed) {
		data["Older"] = "/?page=" + strconv.Itoa(page+1)
	}
	s.render(w, "home.html", data)
}

// catalogPage is where a new agent is chosen: set up for a field of work,
// or blank.
func (s *Server) catalogPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "catalog.html", map[string]any{
		"Title":     "haas — new agent",
		"Nav":       "new",
		"Templates": industry.All(),
		"Refused":   r.URL.Query().Get("refused"),
	})
}

// blankPage starts an agent with nothing set up: a name, and the engine
// and skill set for whoever wants to choose them.
func (s *Server) blankPage(w http.ResponseWriter, r *http.Request) {
	var ready []catalog.Entry
	for _, entry := range s.catalog {
		if entry.Ready {
			ready = append(ready, entry)
		}
	}
	s.render(w, "blank.html", map[string]any{
		"Title":   "haas — new agent",
		"Nav":     "new",
		"Engines": ready,
		"Setups":  s.setupsList(),
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
		agent:   s.agentOf(held),
		Threads: len(s.instances.Threads(held.ID)),
		Where:   threadPath(last),
		Latest:  latest,
		Active:  active,
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
