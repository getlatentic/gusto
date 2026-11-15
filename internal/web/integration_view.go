package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/integration"
)

// integrationView is one imported plugin as its console page shows it: every version, who it is on for, and what was done to it.
// A server's environment and headers are left out, as the page never shows them and they can hold a secret.
type integrationView struct {
	Name string
	// On is the version people get, Before the one Restore turns on again, and Programs the version whose programs may run.
	On, Before, Programs string
	For                  []string
	Versions             []versionRow
	History              []integration.Event
}

// versionRow is one imported version, as a line of the list of versions.
type versionRow struct {
	ID, Label, By string
	Source        integration.Source
	Imported      int64
}

// shownVersion is the version the page opens: what it holds, and what was left out of it.
type shownVersion struct {
	ID, Label, Description string
	Skills                 []shownSkill
	Servers                []shownServer
	Problems               []integration.Problem
}

type shownSkill struct{ Name, Description string }

// shownServer is an MCP server of a version: an address it calls, or a program it brought.
type shownServer struct {
	Name, Transport, URL, Command string
	Args                          []string
}

// versionChanges is what turning the version shown on would change against the version that is on.
type versionChanges struct {
	SkillsAdded, SkillsRemoved, SkillsChanged    []string
	ServersAdded, ServersRemoved, ServersChanged []string
	Manifest                                     []string
	None                                         bool
}

// integrationData is one imported plugin, and the version the address names when it names one.
func (s *Server) integrationData(r *http.Request) (map[string]any, error) {
	held, err := s.integrations.Get(r.PathValue("name"))
	if err != nil {
		return nil, failed{status: http.StatusNotFound, why: err.Error()}
	}
	page := map[string]any{"Integration": viewOfIntegration(held), "Refused": s.refusal(r)}
	if to, found := held.Version(r.URL.Query().Get("version")); found {
		page["Shown"] = showVersion(to)
		if held.On != "" && held.On != to.ID {
			if changes, err := held.Compare(held.On, to.ID); err == nil {
				page["Changes"], page["Against"] = changesOf(changes), held.On
			}
		}
	}
	return page, nil
}

func viewOfIntegration(held *integration.Integration) integrationView {
	view := integrationView{Name: held.Name, On: held.On, Before: held.Before, Programs: held.Programs, For: held.For, History: held.History}
	for _, version := range held.Versions {
		view.Versions = append(view.Versions, versionRow{ID: version.ID, Label: version.Label, By: version.By, Source: version.Source, Imported: version.Imported})
	}
	return view
}

func showVersion(version integration.Version) shownVersion {
	shown := shownVersion{ID: version.ID, Label: version.Label, Description: version.Plugin.Description, Problems: version.Plugin.Problems,
		Skills: []shownSkill{}, Servers: []shownServer{}}
	for _, skill := range version.Plugin.Skills {
		shown.Skills = append(shown.Skills, shownSkill{Name: skill.Name, Description: skill.Description})
	}
	for _, server := range version.Plugin.Servers {
		shown.Servers = append(shown.Servers, shownServer{Name: server.Name, Transport: string(server.Transport), URL: server.URL, Command: server.Command, Args: server.Args})
	}
	return shown
}

func changesOf(c integration.Changes) versionChanges {
	return versionChanges{SkillsAdded: c.SkillsAdded, SkillsRemoved: c.SkillsRemoved, SkillsChanged: c.SkillsChanged,
		ServersAdded: c.ServersAdded, ServersRemoved: c.ServersRemoved, ServersChanged: c.ServersChanged, Manifest: c.Manifest, None: c.None()}
}
