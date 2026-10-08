package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/industry"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/integration"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/setup"
)

// UseSetups hands the pages the store of setups and the MCP servers this
// deployment offers. A server without it offers no setups.
// Each template already installed is brought up to the version this build
// ships, so a fixed skill reaches the agents started from it.
func (s *Server) UseSetups(store *setup.Store, servers setup.Servers) {
	s.setups, s.servers = store, servers
	for _, held := range industry.All() {
		if _, installed := store.Get(held.Name); !installed {
			continue
		}
		if err := s.install(held); err != nil {
			slog.Warn("a template's skills were not brought up to date", "template", held.Name, "error", err)
		}
	}
}

// UseInBuilding tells the pages whether restricted work can run here.
func (s *Server) UseInBuilding(reachable bool) { s.inBuilding.Store(reachable) }

// setupsPath is where the console keeps the skill sets, for whoever changes them.
const setupsPath = "/admin/skill-sets"

// setupsPage is every skill set, for anybody to read.
func (s *Server) setupsPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "setups.html", s.setupsData(r, "/setups"))
}

// skillSetsPage is every skill set, and the way to make another.
func (s *Server) skillSetsPage(w http.ResponseWriter, r *http.Request) {
	s.drawConsole(w, "setups.html", "skill-sets", s.setupsData(r, setupsPath))
}

func (s *Server) setupsData(r *http.Request, base string) map[string]any {
	return map[string]any{
		"Title":    productName + " — skill sets",
		"Nav":      "new",
		"Base":     base,
		"Setups":   s.setupsList(),
		"Offered":  s.servers,
		"Imported": s.importedIntegrations(),
		"Keeps":    s.setups != nil,
		"Refused":  r.URL.Query().Get("refused"),
	}
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
		s.turnAway(w, r, setupsPath, "this server keeps no setups")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.turnAway(w, r, setupsPath, err.Error())
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	wanted := setup.Setup{Name: name, Who: s.keeper.who(r)}
	action := "created"
	if held, found := s.setups.Get(name); found {
		wanted = held
		action = "changed"
	}
	wanted.Servers, wanted.Integrations = r.Form["server"], r.Form["integration"]
	if err := s.setups.Save(wanted, s.servers); err != nil {
		s.turnAway(w, r, setupsPath, err.Error())
		return
	}
	s.note(r, changelog.SetupSaved, map[string]string{"setup": name, "action": action, "servers": strings.Join(wanted.Servers, ", "), "integrations": strings.Join(wanted.Integrations, ", ")})
	http.Redirect(w, r, setupsPath+"/"+name, http.StatusSeeOther)
}

// setupPage is one setup: its skills and its servers, for anybody to read.
func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	if page, ok := s.setupData(w, r, "/setups"); ok {
		s.render(w, "setup-detail.html", page)
	}
}

// skillSetPage is one setup, and the way to change its skills and servers.
func (s *Server) skillSetPage(w http.ResponseWriter, r *http.Request) {
	if page, ok := s.setupData(w, r, setupsPath); ok {
		s.drawConsole(w, "setup-detail.html", "skill-sets", page)
	}
}

// setupData is what a page of the setup the path names is drawn from, and
// answers 404 itself when there is none.
func (s *Server) setupData(w http.ResponseWriter, r *http.Request, base string) (map[string]any, bool) {
	if s.setups == nil {
		http.NotFound(w, r)
		return nil, false
	}
	held, found := s.setups.Get(r.PathValue("name"))
	if !found {
		http.NotFound(w, r)
		return nil, false
	}
	return map[string]any{
		"Title":    productName + " — " + held.Name,
		"Nav":      "new",
		"Base":     base,
		"Setup":    held,
		"Offered":  s.servers,
		"Imported": s.importedIntegrations(),
		"Catalog":  s.entries(),
		"Refused":  r.URL.Query().Get("refused"),
	}, true
}

func (s *Server) writeSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	back := setupsPath + "/" + name
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
	s.note(r, changelog.SkillWritten, map[string]string{"setup": name, "skill": skill.Name})
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (s *Server) dropSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.setups == nil {
		http.NotFound(w, r)
		return
	}
	if err := s.setups.Drop(name, r.PathValue("skill")); err != nil {
		s.turnAway(w, r, setupsPath+"/"+name, err.Error())
		return
	}
	s.note(r, changelog.SkillDropped, map[string]string{"setup": name, "skill": r.PathValue("skill")})
	http.Redirect(w, r, setupsPath+"/"+name, http.StatusSeeOther)
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
	for _, entry := range s.entries() {
		if entry.Name == wanted.Harness {
			if why := held.Fits(entry.Skills, entry.MCP); why != "" {
				return wanted.Harness + " cannot take " + held.Name + ": " + why
			}
		}
	}
	return ""
}

// importedIntegrations is the integrations a setup may name, or none where the server keeps none.
func (s *Server) importedIntegrations() []*integration.Integration {
	if s.integrations == nil {
		return nil
	}
	all, err := s.integrations.List()
	if err != nil {
		slog.Warn("the integrations could not be listed", "error", err)
	}
	return all
}
