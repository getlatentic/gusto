package web

import (
	"net/http"
	"slices"
	"time"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/industry"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/setup"
)

// templatePage is one template: what it is for, what it brings, and which of
// this deployment's harnesses can run it.
func (s *Server) templatePage(w http.ResponseWriter, r *http.Request) {
	held, found := industry.Get(r.PathValue("name"))
	if !found {
		http.NotFound(w, r)
		return
	}
	s.render(w, "template.html", map[string]any{
		"Title":      "haas — " + held.Title,
		"Nav":        "new",
		"Template":   held,
		"Harnesses":  s.runnersOf(held),
		"InBuilding": s.inBuilding,
		"Refused":    r.URL.Query().Get("refused"),
	})
}

// runnersOf is the harnesses here that can take a template: the ones it
// prefers first, in its order, then any other that can.
func (s *Server) runnersOf(held industry.Template) []catalog.Entry {
	takes := func(entry catalog.Entry) bool {
		return entry.Ready && entry.Skills && (len(s.offeredOf(held)) == 0 || entry.MCP)
	}
	var fit []catalog.Entry
	for _, name := range held.Harnesses {
		for _, entry := range s.catalog {
			if entry.Name == name && takes(entry) {
				fit = append(fit, entry)
			}
		}
	}
	for _, entry := range s.catalog {
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
	chosen := r.FormValue("harness")
	if !slices.ContainsFunc(s.runnersOf(held), func(entry catalog.Entry) bool { return entry.Name == chosen }) {
		s.turnAway(w, r, back, "choose a harness this deployment can run the template on")
		return
	}
	if err := s.install(held); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	if held.Restricted() && !s.inBuilding {
		s.turnAway(w, r, back, "this work has to stay in your network, and no model here is")
		return
	}
	wanted := instance.Instance{
		Title:   held.Title,
		Harness: chosen, Model: s.models.First(chosen), Setup: held.Name,
		Sensitivity: string(held.Sensitivity), Who: s.keeper.who(r),
	}
	started, err := s.instances.Start(s.index.Free(held.Name, time.Now()), wanted)
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

// install keeps a template as a setup of the same name. One already there is
// left as it is, because somebody may have changed its skills since.
func (s *Server) install(held industry.Template) error {
	if s.setups == nil {
		return errNoSetups
	}
	if _, found := s.setups.Get(held.Name); found {
		return nil
	}
	if err := s.setups.Save(setup.Setup{Name: held.Name, Servers: s.offeredOf(held), Starters: held.Starters, Who: "haas"}, s.servers); err != nil {
		return err
	}
	for _, skill := range held.Skills {
		if err := s.setups.Write(held.Name, setup.Skill{Name: skill.Name, Description: skill.Description, Body: skill.Body}); err != nil {
			return err
		}
	}
	return nil
}

// startersOf is what an empty thread on this instance offers to say.
func (s *Server) startersOf(held instance.Instance) []string {
	if held.Setup == "" || s.setups == nil {
		return nil
	}
	kept, _ := s.setups.Get(held.Setup)
	return kept.Starters
}
