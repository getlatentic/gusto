package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/tasks"
)

// yours is an instance as the home page lists it: what it is, what was last
// said to it, and where to pick that conversation up.
type yours struct {
	agent
	Threads int
	Where   string
	Latest  tasks.Record
	// Active is when it was last spoken to, or started when it never was.
	Active int64
}

// presetsData is where a new agent is chosen: the person's own presets, then each ready-made kind of work and the fields of work
// they come under, in order.
func (s *Server) presetsData(r *http.Request) (map[string]any, error) {
	roles, fields := s.rolesToChoose()
	return map[string]any{
		"Roles":   roles,
		"Fields":  fields,
		"Presets": orNone(s.presetRows(r)),
		"Refused": s.refusal(r),
	}, nil
}

// role is a ready-made kind of work as the presets page offers it: what it does, what to try asking, and why it cannot be started
// here when it cannot.
type role struct {
	Name, Title, Industry, Summary string
	Skills                         []roleSkill
	Starters                       []string
	Blocked                        string
}

// roleSkill is what one of a role's skills does, in the words of the person choosing.
type roleSkill struct{ Title, Blurb string }

// rolesToChoose is every template with what is needed to start it, and the fields of work they come under, in order.
func (s *Server) rolesToChoose() ([]role, []string) {
	roles := []role{}
	fields := []string{}
	seen := map[string]bool{}
	for _, held := range industry.All() {
		each := role{Name: held.Name, Title: held.Title, Industry: held.Industry, Summary: held.Summary, Skills: []roleSkill{},
			Starters: orNone(held.Starters), Blocked: s.blocked(held)}
		for _, skill := range held.Skills {
			each.Skills = append(each.Skills, roleSkill{Title: skill.Title, Blurb: skill.Blurb})
		}
		roles = append(roles, each)
		if !seen[held.Industry] {
			seen[held.Industry] = true
			fields = append(fields, held.Industry)
		}
	}
	return roles, fields
}

// blocked is why a template cannot be started here, or nothing when it can.
func (s *Server) blocked(held industry.Template) string {
	switch {
	case held.Restricted() && !s.inBuilding.Load():
		return "This work only goes to models in your own network, and this deployment has none."
	case len(s.runnersOf(held)) == 0:
		return "No agent here can take this work yet."
	}
	return ""
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

// blankEngine is an agent type a blank agent may be started as.
type blankEngine struct{ Name, Title, What string }

// blankData starts an agent with nothing set up: a name, and the engine and skill set for whoever wants to choose them.
func (s *Server) blankData(r *http.Request) (map[string]any, error) {
	engines := []blankEngine{}
	for _, entry := range s.readyEngines() {
		engines = append(engines, blankEngine{Name: entry.Name, Title: entry.Title, What: entry.What})
	}
	setups := []string{}
	for _, held := range s.setupsList() {
		setups = append(setups, held.Name)
	}
	return map[string]any{"Engines": engines, "Setups": setups, "Refused": s.refusal(r)}, nil
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
