package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
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

// catalogPage is where a new agent is chosen: set up for a field of work,
// or blank.
func (s *Server) catalogPage(w http.ResponseWriter, r *http.Request) {
	roles, fields := s.rolesToChoose()
	s.render(w, "catalog.html", map[string]any{
		"Title":   productName + " — presets",
		"Nav":     "new",
		"Roles":   roles,
		"Fields":  fields,
		"Presets": s.presetRows(r),
		"Refused": r.URL.Query().Get("refused"),
	})
}

// role is a ready-made kind of work as the new-agent page offers it: what it does, and whether it can be started here.
type role struct {
	industry.Template
	Fit     []catalog.Entry
	Blocked string
}

// rolesToChoose is every template with what is needed to start it, and the fields of work they come under, in order.
func (s *Server) rolesToChoose() ([]role, []string) {
	var roles []role
	var fields []string
	seen := map[string]bool{}
	for _, held := range industry.All() {
		runners := s.runnersOf(held)
		each := role{Template: held, Fit: runners}
		switch {
		case held.Restricted() && !s.inBuilding.Load():
			each.Blocked = "This work only goes to models in your own network, and this deployment has none."
		case len(runners) == 0:
			each.Blocked = "No agent here can take this work yet."
		}
		roles = append(roles, each)
		if !seen[held.Industry] {
			seen[held.Industry] = true
			fields = append(fields, held.Industry)
		}
	}
	return roles, fields
}

// readyEngines is the agents this deployment can start now.
func (s *Server) readyEngines() []catalog.Entry {
	var ready []catalog.Entry
	for _, entry := range s.engines() {
		if entry.Ready {
			ready = append(ready, entry)
		}
	}
	return ready
}

// blankPage starts an agent with nothing set up: a name, and the engine
// and skill set for whoever wants to choose them.
func (s *Server) blankPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "blank.html", map[string]any{
		"Title":   productName + " — new agent",
		"Nav":     "new",
		"Engines": s.readyEngines(),
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
	held, found := s.mine(w, r)
	if !found {
		return
	}
	id := held.ID
	for _, record := range s.index.List() {
		if record.Instance == id && !record.State.Done() {
			s.turnAway(w, r, "/instances/"+id, "a turn is still "+string(record.State)+" here; remove it once that ends")
			return
		}
	}
	for _, each := range s.appsOf(id) {
		s.stopApp(each)
	}
	if err := s.instances.Remove(id); err != nil {
		s.turnAway(w, r, "/instances/"+id, err.Error())
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
