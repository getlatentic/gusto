package web

import (
	"cmp"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/internal/team"
)

// templateView is a template as its page shows it: what it is for, the skills it brings with their instructions drawn from their
// Markdown, what to try asking, and the names of its sample files.
type templateView struct {
	Name, Title, Industry, Summary string
	Restricted                     bool
	Skills                         []templateSkill
	Starters                       []string
	Files                          []string
}

// templateSkill is one of a template's skills. Instructions is its body as HTML, drawn as an agent's reply is, so none of it runs.
type templateSkill struct {
	Title, Blurb, Instructions string
}

// templateRunner is an agent type a template can be started on.
type templateRunner struct{ Name, Title string }

// hasTemplate is whether the address names a template.
func (s *Server) hasTemplate(r *http.Request) bool {
	_, found := industry.Get(r.PathValue("name"))
	return found
}

// templateData is one template: what it is for, what it brings, and which of this deployment's harnesses can run it, the one
// it starts on first. InBuilding is whether a model inside the deployment's own network is reachable, which restricted work needs.
func (s *Server) templateData(r *http.Request) (map[string]any, error) {
	held, found := industry.Get(r.PathValue("name"))
	if !found {
		return nil, failed{http.StatusNotFound, "there is no such template"}
	}
	runners := []templateRunner{}
	for _, entry := range s.runnersOf(held) {
		runners = append(runners, templateRunner{Name: entry.Name, Title: entry.Title})
	}
	return map[string]any{
		"Template":   viewOfTemplate(held),
		"Harnesses":  runners,
		"InBuilding": s.inBuilding.Load(),
		"Refused":    s.refusal(r),
	}, nil
}

func viewOfTemplate(held industry.Template) templateView {
	view := templateView{Name: held.Name, Title: held.Title, Industry: held.Industry, Summary: held.Summary, Restricted: held.Restricted(),
		Skills: []templateSkill{}, Starters: orNone(held.Starters), Files: slices.Sorted(maps.Keys(held.Files))}
	for _, skill := range held.Skills {
		view.Skills = append(view.Skills, templateSkill{Title: skill.Title, Blurb: skill.Blurb, Instructions: string(markdown(skill.Body))})
	}
	view.Files = orNone(view.Files)
	return view
}

// runnersOf is the harnesses here that can take a template: the ones it
// prefers first, in its order, then any other that can.
func (s *Server) runnersOf(held industry.Template) []catalog.Entry {
	takes := func(entry catalog.Entry) bool {
		return entry.Ready && entry.Skills && (len(s.offeredOf(held)) == 0 || entry.MCP)
	}
	var fit []catalog.Entry
	for _, name := range held.Harnesses {
		for _, entry := range s.engines() {
			if entry.Name == name && takes(entry) {
				fit = append(fit, entry)
			}
		}
	}
	for _, entry := range s.engines() {
		if takes(entry) && !slices.Contains(held.Harnesses, entry.Name) {
			fit = append(fit, entry)
		}
	}
	return fit
}

// offeredOf is the template's servers this deployment offers. A template asks
// for a server; a deployment that does not offer it runs the template without.
func (s *Server) offeredOf(held industry.Template) []string {
	var offered []string
	for _, name := range held.Servers {
		if _, found := s.servers[name]; found {
			offered = append(offered, name)
		}
	}
	return offered
}

// startTemplate gives a harness the template's setup and sample folder, and
// opens a conversation with it.
func (s *Server) startTemplate(w http.ResponseWriter, r *http.Request) {
	held, found := industry.Get(r.PathValue("name"))
	if !found {
		http.NotFound(w, r)
		return
	}
	back := "/templates/" + held.Name
	chosen := cmp.Or(r.FormValue("harness"), r.FormValue("engine"))
	if fit := s.runnersOf(held); chosen == "" && len(fit) > 0 {
		chosen = fit[0].Name
	}
	if !slices.ContainsFunc(s.runnersOf(held), func(entry catalog.Entry) bool { return entry.Name == chosen }) {
		s.turnAway(w, r, back, "choose an agent this deployment can run the template on")
		return
	}
	if err := s.install(held); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	if held.Restricted() && !s.inBuilding.Load() {
		s.turnAway(w, r, back, "this work has to stay in your network, and no model here is")
		return
	}
	if s.limitedAway(r) {
		s.turnAway(w, r, back, limitedWords)
		return
	}
	wanted := instance.Instance{
		Title:   held.Title,
		Harness: chosen, Model: first(s.offeredTo(r)), Setup: held.Name,
		Sensitivity: string(held.Sensitivity), Who: s.keeper.who(r), Team: team.Stored(s.teamOf(r)),
	}
	started, err := s.instances.Start(tasks.Name(held.Name, time.Now()), wanted)
	if err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	if err := s.instances.Seed(started.ID, held.Files); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	http.Redirect(w, r, threadPath(s.instances.Latest(started.ID)), http.StatusSeeOther)
}

// install keeps a template as a setup of the same name, and brings one
// already there up to the template's version wherever nobody has changed it.
func (s *Server) install(held industry.Template) error {
	if s.setups == nil {
		return errNoSetups
	}
	skills := make([]setup.Skill, len(held.Skills))
	for i, skill := range held.Skills {
		skills[i] = setup.Skill{Name: skill.Name, Description: skill.Description, Body: skill.Body}
	}
	return s.setups.Provide(setup.Setup{Name: held.Name, Servers: s.offeredOf(held), Starters: held.Starters, Skills: skills}, s.servers)
}

// startersOf is what an empty thread on this instance offers to say.
func (s *Server) startersOf(held instance.Instance) []string {
	if held.Setup == "" || s.setups == nil {
		return nil
	}
	kept, _ := s.setups.Get(held.Setup)
	return kept.Starters
}
