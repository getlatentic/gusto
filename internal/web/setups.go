package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/integration"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/pkg/job"
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
	list := s.setupsList()
	if base == setupsPath {
		list = s.setupsAll()
	}
	return map[string]any{
		"Title":    productName + " — skill sets",
		"Nav":      "presets",
		"Base":     base,
		"Setups":   list,
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

// sensitivityOf is what a turn on this instance is labeled: the instance's
// own, or internal.
func sensitivityOf(held instance.Instance) job.Sensitivity {
	if held.Sensitivity == "" {
		return job.Internal
	}
	return job.Sensitivity(held.Sensitivity)
}

// equip carries the setup a job names into its directory, and puts in
// job.json what it actually held, whatever the sender said it held.
func (s *Server) equip(spec *job.Spec, dir job.Dir, asker string) error {
	if spec.Setup == nil {
		return nil
	}
	if s.setups == nil {
		return errNoSetups
	}
	carried, err := s.setups.CarryFor(spec.Setup.Name, s.servers, dir, asker)
	if err != nil {
		return err
	}
	spec.Setup = carried
	return nil
}

var errNoSetups = errors.New("this server keeps no setups")

// setupsList is the setups people are offered: every one that is on.
func (s *Server) setupsList() []setup.Setup {
	var on []setup.Setup
	for _, held := range s.setupsAll() {
		if !held.Off {
			on = append(on, held)
		}
	}
	return on
}

// setupsAll is every setup, off ones too, for the console.
func (s *Server) setupsAll() []setup.Setup {
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
	// A setup an administrator makes is off until an administrator turns it on, as a provider or an integration is.
	wanted := setup.Setup{Name: name, Who: s.keeper.who(r), Off: true}
	action := "created"
	was := map[string]string{}
	if held, found := s.setups.Get(name); found {
		wanted = held
		action = "changed"
		was["servers"], was["integrations"] = strings.Join(held.Servers, ", "), strings.Join(held.Integrations, ", ")
	}
	wanted.Servers, wanted.Integrations = r.Form["server"], r.Form["integration"]
	if err := s.setups.Save(wanted, s.servers); err != nil {
		s.turnAway(w, r, setupsPath, err.Error())
		return
	}
	s.note(r, changelog.SetupSaved, map[string]string{"setup": name, "action": action, "servers": strings.Join(wanted.Servers, ", "), "integrations": strings.Join(wanted.Integrations, ", "),
		"servers was": was["servers"], "integrations was": was["integrations"], "off": strconv.FormatBool(wanted.Off)})
	http.Redirect(w, r, setupsPath+"/"+name, http.StatusSeeOther)
}

// setupPage is one setup: its skills and its servers, for anybody to read.
func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	if page, ok := s.setupData(w, r, "/setups"); ok {
		s.render(w, "setup-detail.html", page)
	}
}

// exportSetup is a skill set as an Agent Plugin, for another deployment to import, or this one.
func (s *Server) exportSetup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	data, err := s.setups.Export(name)
	if err != nil {
		s.turnAway(w, r, setupsPath+"/"+name, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`-plugin.zip"`)
	w.Write(data)
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
	if !found || (held.Off && base != setupsPath) {
		http.NotFound(w, r)
		return nil, false
	}
	return map[string]any{
		"Earlier":  s.setups.Earlier(held.Name),
		"Title":    productName + " — " + held.Name,
		"Nav":      "presets",
		"Base":     base,
		"Setup":    held,
		"Offered":  s.servers,
		"Imported": s.importedIntegrations(),
		"Catalog":  s.engines(),
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
	was := s.setups.Fingerprint(name, skill.Name)
	if err := s.setups.Write(name, skill); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	s.note(r, changelog.SkillWritten, map[string]string{"setup": name, "skill": skill.Name, "was": was, "now": s.setups.Fingerprint(name, skill.Name)})
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// restoreSkill puts a kept version of a skill back.
func (s *Server) restoreSkill(w http.ResponseWriter, r *http.Request) {
	name, skill := r.PathValue("name"), r.PathValue("skill")
	back := setupsPath + "/" + name
	if s.setups == nil {
		http.NotFound(w, r)
		return
	}
	at, err := strconv.ParseInt(r.FormValue("at"), 10, 64)
	if err != nil {
		s.turnAway(w, r, back, "say which version to put back")
		return
	}
	was := s.setups.Fingerprint(name, skill)
	if err := s.setups.RestoreSkill(name, skill, at); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	s.note(r, changelog.SkillRestored, map[string]string{"setup": name, "skill": skill, "was": was, "now": s.setups.Fingerprint(name, skill)})
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// turnSetup turns a setup on, which is what makes what an administrator made available to people, or off.
func (s *Server) turnSetup(off bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if s.setups == nil {
			http.NotFound(w, r)
			return
		}
		if err := s.setups.SetOff(name, off); err != nil {
			s.turnAway(w, r, setupsPath+"/"+name, err.Error())
			return
		}
		kind, was, now := changelog.SetupEnabled, "off", "on"
		if off {
			kind, was, now = changelog.SetupDisabled, "on", "off"
		}
		s.note(r, kind, map[string]string{"setup": name, "was": was, "now": now})
		http.Redirect(w, r, setupsPath+"/"+name, http.StatusSeeOther)
	}
}

func (s *Server) dropSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.setups == nil {
		http.NotFound(w, r)
		return
	}
	was := s.setups.Fingerprint(name, r.PathValue("skill"))
	if err := s.setups.Drop(name, r.PathValue("skill")); err != nil {
		s.turnAway(w, r, setupsPath+"/"+name, err.Error())
		return
	}
	s.note(r, changelog.SkillDropped, map[string]string{"setup": name, "skill": r.PathValue("skill"), "was": was, "now": "none"})
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
	if held.Off {
		return "the setup " + wanted.Setup + " is off: an administrator turns it on"
	}
	for _, entry := range s.engines() {
		if entry.Name == wanted.Harness {
			if why := held.Fits(entry.Skills, entry.MCP); why != "" {
				return unfitSaid(entry.Title, setupTitle(wanted, held), why)
			}
		}
	}
	return ""
}

// setupTitle is what a person calls the setup an instance starts with: the preset's title when it came from one, else the setup's name.
func setupTitle(wanted instance.Instance, held setup.Setup) string {
	if template, found := industry.Get(held.Name); found {
		return template.Title
	}
	if wanted.Title != "" && wanted.Setup == held.Name {
		return wanted.Title
	}
	return held.Name
}

// unfitSaid is why an agent cannot start with a setup, in the words of the person who chose them.
func unfitSaid(agent, work, why string) string {
	needs := "skills"
	if strings.Contains(why, "MCP") {
		needs = "tool servers"
	}
	return agent + " cannot run " + work + ", which needs " + needs + " " + agent + " does not take. Choose another agent."
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
