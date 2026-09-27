package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/setup"
)

// UseSetups hands the pages the store of setups and the MCP servers this
// deployment offers. A server without it offers no setups.
func (s *Server) UseSetups(store *setup.Store, servers setup.Servers) {
	s.setups, s.servers = store, servers
}

// setupOf is what a job on this instance is given, named for job.json.
func setupOf(held instance.Instance) *job.Setup {
	if held.Setup == "" {
		return nil
	}
	return &job.Setup{Name: held.Setup}
}

// sensitivityOf is what a turn on this instance is labelled: the instance's
// own, or internal.
func sensitivityOf(held instance.Instance) job.Sensitivity {
	if held.Sensitivity == "" {
		return job.Internal
	}
	return job.Sensitivity(held.Sensitivity)
}

// equip carries the setup a job names into its directory, and puts in
// job.json what it actually held, whatever the sender said it held.
func (s *Server) equip(spec *job.Spec, dir job.Dir) error {
	if spec.Setup == nil {
		return nil
	}
	if s.setups == nil {
		return errNoSetups
	}
	carried, err := s.setups.Carry(spec.Setup.Name, s.servers, dir)
	if err != nil {
		return err
	}
	spec.Setup = carried
	return nil
}

var errNoSetups = errors.New("this server keeps no setups")

// setupsList is every setup, for a page to offer.
func (s *Server) setupsList() []setup.Setup {
	if s.setups == nil {
		return nil
	}
	return s.setups.List()
}

// saveSetup makes a setup, or changes which servers one names.
func (s *Server) saveSetup(w http.ResponseWriter, r *http.Request) {
	if s.setups == nil {
		s.turnAway(w, r, "/new", "this server keeps no setups")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.turnAway(w, r, "/new", err.Error())
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	wanted := setup.Setup{Name: name, Servers: r.Form["server"], Who: s.keeper.who(r)}
	if held, found := s.setups.Get(name); found {
		wanted.Made, wanted.Who = held.Made, held.Who
	}
	if err := s.setups.Save(wanted, s.servers); err != nil {
		s.turnAway(w, r, "/new", err.Error())
		return
	}
	http.Redirect(w, r, "/setups/"+name, http.StatusSeeOther)
}

// setupPage is one setup: its skills, its servers, and the way to change both.
func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	if s.setups == nil {
		http.NotFound(w, r)
		return
	}
	held, found := s.setups.Get(r.PathValue("name"))
	if !found {
		http.NotFound(w, r)
		return
	}
	s.render(w, "setup-detail.html", map[string]any{
		"Title":   "haas — " + held.Name,
		"Nav":     "new",
		"Setup":   held,
		"Offered": s.servers,
		"Catalog": s.catalog,
		"Refused": r.URL.Query().Get("refused"),
	})
}

func (s *Server) writeSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	back := "/setups/" + name
	if s.setups == nil {
		http.NotFound(w, r)
		return
	}
	skill := setup.Skill{
		Name:        strings.TrimSpace(r.FormValue("skill")),
		Description: r.FormValue("description"),
		Body:        r.FormValue("body"),
	}
	if err := s.setups.Write(name, skill); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (s *Server) dropSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.setups == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.setups.Drop(name, r.PathValue("skill")); err != nil {
		s.turnAway(w, r, "/setups/"+name, err.Error())
		return
	}
	http.Redirect(w, r, "/setups/"+name, http.StatusSeeOther)
}

// fitting is the setups a harness can take, so the form offers no setup the
// runner would refuse.
func (s *Server) fitting(entry catalog.Entry) []setup.Setup {
	var fit []setup.Setup
	for _, held := range s.setupsList() {
		if held.Fits(entry.Skills, entry.MCP) == "" {
			fit = append(fit, held)
		}
	}
	return fit
}

// unfit says why an instance cannot be started with the setup it names.
func (s *Server) unfit(wanted instance.Instance) string {
	if wanted.Setup == "" {
		return ""
	}
	if s.setups == nil {
		return "this server keeps no setups"
	}
	held, found := s.setups.Get(wanted.Setup)
	if !found {
		return "no setup called " + wanted.Setup
	}
	for _, entry := range s.catalog {
		if entry.Name == wanted.Harness {
			if why := held.Fits(entry.Skills, entry.MCP); why != "" {
				return wanted.Harness + " cannot take " + held.Name + ": " + why
			}
		}
	}
	return ""
}
