package web

import (
	"fmt"
	"net/http"
	"slices"
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

// appsData is the tab of Apps & skills the request names: the apps an admin turns on for everyone, or the skills agents are given.
func (s *Server) appsData(r *http.Request) (map[string]any, error) {
	var tabs []tab
	if s.hasConnectors() {
		tabs = append(tabs, tab{Key: "apps", Label: "Apps"})
	}
	if s.hasSkills() {
		tabs = append(tabs, tab{Key: "skills", Label: "Skills"})
	}
	tabs, chosen := chooseTab(r, appsPath, tabs)
	page := map[string]any{"Tabs": tabs, "Tab": chosen, "Refused": s.refusal(r)}
	if chosen == "apps" {
		rows, err := s.appRows()
		if err != nil {
			return nil, err
		}
		page["Apps"], page["TurnedOn"] = rows, s.message(r, "on")
		return page, nil
	}
	imported := s.importedIntegrations()
	page["Skills"], page["Writes"], page["Imports"] = s.skillRows(imported), s.hasSetups(), s.hasIntegrations()
	page["Servers"], page["Imported"] = s.serverNames(), importedChoices(imported, nil)
	return page, nil
}

// appRow is one app of the catalog as the admin's page shows it: whether it is on for everyone, and whether it can be.
type appRow struct {
	ID, Name, What string
	On, Usable     bool
	// NeedSignIn is how many people's sign-ins need making again.
	NeedSignIn int
}

func (s *Server) appRows() ([]appRow, error) {
	views, err := s.connectorViews("", "")
	if err != nil {
		return nil, err
	}
	rows := make([]appRow, 0, len(views))
	for _, view := range views {
		rows = append(rows, appRow{ID: view.ID, Name: view.Name, What: view.What, On: view.On, Usable: view.Usable, NeedSignIn: view.NeedSignIn})
	}
	return rows, nil
}

// skillRow is a skill set written here or a plugin imported, as one line of the skills list.
type skillRow struct {
	Name, Href, Kind, Detail string
	On                       bool
}

func (s *Server) skillRows(imported []*integration.Integration) []skillRow {
	rows := []skillRow{}
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

// serverNames is the MCP servers this deployment offers, by name, in order.
func (s *Server) serverNames() []string {
	names := make([]string, 0, len(s.servers))
	for name := range s.servers {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// importedChoice is an imported plugin a skill set may be given, and whether the one shown is.
type importedChoice struct {
	Name       string
	On, Chosen bool
}

func importedChoices(imported []*integration.Integration, chosen []string) []importedChoice {
	choices := make([]importedChoice, 0, len(imported))
	for _, held := range imported {
		choices = append(choices, importedChoice{Name: held.Name, On: held.On != "", Chosen: slices.Contains(chosen, held.Name)})
	}
	return choices
}
