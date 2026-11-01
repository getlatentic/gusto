package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/integration"
)

// appsPath is the console's one page for what agents may use: the apps people connect, and the skills they are given.
const appsPath = "/admin/apps"

const (
	appsTab   = appsPath + "?tab=apps"
	skillsTab = appsPath + "?tab=skills"
)

func (s *Server) hasAppsOrSkills() bool { return s.hasConnectors() || s.hasSkills() }

func (s *Server) hasSkills() bool { return s.hasSetups() || s.hasIntegrations() }

func (s *Server) appsPage(w http.ResponseWriter, r *http.Request) {
	var tabs []tab
	if s.hasConnectors() {
		tabs = append(tabs, tab{Key: "apps", Label: "Apps"})
	}
	if s.hasSkills() {
		tabs = append(tabs, tab{Key: "skills", Label: "Skills"})
	}
	tabs, chosen := chooseTab(r, appsPath, tabs)
	page := map[string]any{
		"Title": productName + " — apps & skills", "Tabs": tabs, "Tab": chosen, "Refused": r.URL.Query().Get("refused"),
	}
	if chosen == "apps" {
		views, err := s.connectorViews("")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		page["Connectors"], page["TurnedOn"] = views, r.URL.Query().Get("on")
	} else {
		imported := s.importedIntegrations()
		page["Skills"], page["Writes"], page["Imports"] = s.skillRows(imported), s.hasSetups(), s.hasIntegrations()
		page["Offered"], page["Imported"] = s.servers, imported
	}
	s.drawConsole(w, "admin-apps.html", "apps", page)
}

// skillRow is a skill set written here or a plugin imported, as one line of the skills list.
type skillRow struct {
	Name, Href, Kind, Detail string
	On                       bool
}

func (s *Server) skillRows(imported []*integration.Integration) []skillRow {
	var rows []skillRow
	for _, held := range s.setupsAll() {
		detail := fmt.Sprintf("%d %s", len(held.Skills), plural(len(held.Skills), "skill"))
		if len(held.Servers) > 0 {
			detail += " · tools: " + strings.Join(held.Servers, ", ")
		}
		rows = append(rows, skillRow{Name: held.Name, Href: setupsPath + "/" + held.Name, Kind: "Skill set", Detail: detail, On: !held.Off})
	}
	for _, held := range imported {
		rows = append(rows, skillRow{Name: held.Name, Href: integrationsPath + "/" + held.Name, Kind: "Imported plugin",
			Detail: fmt.Sprintf("%d %s", len(held.Versions), plural(len(held.Versions), "version")), On: held.On != ""})
	}
	return rows
}
