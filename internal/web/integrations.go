package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/integration"
)

// integrationsPath is where the console keeps the integrations an organisation has imported.
const integrationsPath = "/admin/integrations"

// UseIntegrations hands the pages the store of imported integrations and the way to fetch one from a git address. A server without
// it offers none.
func (s *Server) UseIntegrations(store *integration.Store, clone integration.Clone) {
	s.integrations, s.cloner = store, clone
}

func (s *Server) hasIntegrations() bool { return s.integrations != nil }

// importLimit is how much of a request the import takes: an upload, and the form around it.
const importLimit = integration.MaxUpload + 1<<20

func (s *Server) integrationRoutes(mux *http.ServeMux) {
	offered := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasIntegrations, next)) }
	mux.HandleFunc("GET "+integrationsPath, offered(s.integrationsPage))
	mux.HandleFunc("POST "+integrationsPath+"/import", offered(s.importIntegration))
	mux.HandleFunc("GET "+integrationsPath+"/{name}", offered(s.integrationPage))
	mux.HandleFunc("POST "+integrationsPath+"/{name}/enable", offered(s.enableIntegration))
	mux.HandleFunc("POST "+integrationsPath+"/{name}/disable", offered(s.disableIntegration))
	mux.HandleFunc("POST "+integrationsPath+"/{name}/restore", offered(s.restoreIntegration))
	mux.HandleFunc("POST "+integrationsPath+"/{name}/programs/allow", offered(s.allowPrograms))
	mux.HandleFunc("POST "+integrationsPath+"/{name}/programs/refuse", offered(s.refusePrograms))
}

func (s *Server) integrationsPage(w http.ResponseWriter, r *http.Request) {
	all, err := s.integrations.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.drawConsole(w, "admin-integrations.html", "integrations", map[string]any{
		"Title":        productName + " — integrations",
		"Integrations": all,
		"Refused":      r.URL.Query().Get("refused"),
	})
}

func (s *Server) integrationPage(w http.ResponseWriter, r *http.Request) {
	held, err := s.integrations.Get(r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	page := map[string]any{
		"Title":   productName + " — " + held.Name,
		"Held":    held,
		"Refused": r.URL.Query().Get("refused"),
	}
	if to, found := held.Version(r.URL.Query().Get("version")); found {
		page["Shown"] = to
		if held.On != "" && held.On != to.ID {
			if changes, err := held.Compare(held.On, to.ID); err == nil {
				page["Changes"], page["Against"] = changes, held.On
			}
		}
	}
	s.drawConsole(w, "admin-integration.html", "integrations", page)
}

// importIntegration takes a plugin in, from an upload or a git address, as a version that is off.
func (s *Server) importIntegration(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		s.turnAway(w, r, integrationsPath, "the upload could not be read: "+err.Error())
		return
	}
	scratch, err := os.MkdirTemp("", "haas-integration-")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(scratch)
	dir, source, err := s.fetchIntegration(r, scratch)
	if err != nil {
		s.turnAway(w, r, integrationsPath, err.Error())
		return
	}
	held, version, err := s.integrations.Import(dir, source, s.keeper.who(r))
	if err != nil {
		s.turnAway(w, r, integrationsPath, err.Error())
		return
	}
	s.note(r, changelog.IntegrationImported, map[string]string{
		"integration": held.Name, "version": version.ID, "label": version.Label, "source": source.Kind, "address": source.Address, "commit": source.Commit,
	})
	http.Redirect(w, r, integrationsPath+"/"+held.Name+"?version="+version.ID, http.StatusSeeOther)
}

// fetchIntegration is the plugin the form names, as a folder.
func (s *Server) fetchIntegration(r *http.Request, scratch string) (string, integration.Source, error) {
	if r.FormValue("kind") == "git" {
		return s.cloner.Fetch(r.Context(), strings.TrimSpace(r.FormValue("address")), strings.TrimSpace(r.FormValue("ref")),
			strings.Trim(strings.TrimSpace(r.FormValue("folder")), "/"), scratch)
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		return "", integration.Source{}, errors.New("choose a .zip or a .tar.gz file, or give a git address")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, integration.MaxUpload+1))
	if err != nil {
		return "", integration.Source{}, err
	}
	dir, err := integration.Unpack(data, scratch)
	return dir, integration.Source{Kind: "upload", Address: header.Filename}, err
}

func (s *Server) enableIntegration(w http.ResponseWriter, r *http.Request) {
	name, version := r.PathValue("name"), r.FormValue("version")
	before, err := s.integrations.Get(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	changes := ""
	if before.On != "" && before.On != version {
		if difference, err := before.Compare(before.On, version); err == nil {
			changes = describeChanges(difference)
		}
	}
	held, err := s.integrations.Enable(name, version, s.keeper.who(r))
	if err != nil {
		s.turnAway(w, r, integrationsPath+"/"+name, err.Error())
		return
	}
	s.note(r, changelog.IntegrationEnabled, map[string]string{"integration": name, "from": before.On, "to": held.On, "changes": changes})
	http.Redirect(w, r, integrationsPath+"/"+name, http.StatusSeeOther)
}

func (s *Server) disableIntegration(w http.ResponseWriter, r *http.Request) {
	s.turnIntegration(w, r, changelog.IntegrationDisabled, s.integrations.Disable)
}

func (s *Server) restoreIntegration(w http.ResponseWriter, r *http.Request) {
	s.turnIntegration(w, r, changelog.IntegrationRestored, s.integrations.Restore)
}

// turnIntegration does a change that names no version, and records what was on and what is.
func (s *Server) turnIntegration(w http.ResponseWriter, r *http.Request, kind string, do func(name, by string) (*integration.Integration, error)) {
	name := r.PathValue("name")
	before, err := s.integrations.Get(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	held, err := do(name, s.keeper.who(r))
	if err != nil {
		s.turnAway(w, r, integrationsPath+"/"+name, err.Error())
		return
	}
	s.note(r, kind, map[string]string{"integration": name, "from": before.On, "to": held.On})
	http.Redirect(w, r, integrationsPath+"/"+name, http.StatusSeeOther)
}

// allowPrograms lets the programs of one version run, in a box of their own. It is a separate step from turning the version on, and it
// is for that version only.
func (s *Server) allowPrograms(w http.ResponseWriter, r *http.Request) {
	name, version := r.PathValue("name"), r.FormValue("version")
	before, err := s.integrations.Get(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.FormValue("understood") != "yes" {
		s.turnAway(w, r, integrationsPath+"/"+name, "tick that you have read what the programs are, and that they run in a box that has no network unless this server was set up to give it one")
		return
	}
	held, err := s.integrations.AllowPrograms(name, version, s.keeper.who(r))
	if err != nil {
		s.turnAway(w, r, integrationsPath+"/"+name, err.Error())
		return
	}
	s.note(r, changelog.IntegrationPrograms, map[string]string{"integration": name, "allowed": held.Programs, "was": before.Programs})
	http.Redirect(w, r, integrationsPath+"/"+name, http.StatusSeeOther)
}

func (s *Server) refusePrograms(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	before, err := s.integrations.Get(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	held, err := s.integrations.RefusePrograms(name, s.keeper.who(r))
	if err != nil {
		s.turnAway(w, r, integrationsPath+"/"+name, err.Error())
		return
	}
	s.note(r, changelog.IntegrationPrograms, map[string]string{"integration": name, "allowed": held.Programs, "was": before.Programs})
	http.Redirect(w, r, integrationsPath+"/"+name, http.StatusSeeOther)
}

// describeChanges is what differs between two versions, in a line for the record.
func describeChanges(c integration.Changes) string {
	var parts []string
	for _, part := range []struct {
		label string
		names []string
	}{
		{"skills added", c.SkillsAdded}, {"skills removed", c.SkillsRemoved}, {"skills changed", c.SkillsChanged},
		{"servers added", c.ServersAdded}, {"servers removed", c.ServersRemoved}, {"servers changed", c.ServersChanged}, {"manifest", c.Manifest},
	} {
		if len(part.names) > 0 {
			parts = append(parts, fmt.Sprintf("%s: %s", part.label, strings.Join(part.names, ", ")))
		}
	}
	if len(parts) == 0 {
		return "no difference in skills, servers or manifest"
	}
	return strings.Join(parts, "; ")
}
