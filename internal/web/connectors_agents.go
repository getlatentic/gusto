package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/changelog"
)

// agentApps is what an agent's Apps tab shows the person looking: each app they connected, and what the agent may do with it.
type agentApps struct {
	Apps []agentApp
	// Person is false for the shared sign-in, which is nobody and has no apps to give.
	Person bool
}

// agentApp is one app of the person's: what they chose for this agent, inherit, allow or deny, and their default for their agents,
// which inherit follows.
type agentApp struct {
	ID, Name, Choice string
	Default          bool
}

// agentAppsFor is an agent's Apps tab for the person asking, or nil where this server offers no apps.
func (s *Server) agentAppsFor(r *http.Request, member string) *agentApps {
	if !s.hasConnectors() {
		return nil
	}
	who, person := s.connectingPerson(r)
	shown := &agentApps{Person: person}
	if !person {
		return shown
	}
	views, err := s.connectorViews(who)
	if err != nil {
		return shown
	}
	held, _ := s.crew.Get(member)
	for _, view := range views {
		if !view.On || !view.Usable || !view.Connected {
			continue
		}
		app := agentApp{ID: view.ID, Name: view.Name, Choice: "inherit", Default: s.connectors.ForAgents(who, view.ID)}
		if allowed, chosen := held.AppChoice(who, view.ID); chosen {
			app.Choice = map[bool]string{true: "allow", false: "deny"}[allowed]
		}
		shown.Apps = append(shown.Apps, app)
	}
	return shown
}

// chooseAgentApp sets what an agent may do with one app of the asking person's: inherit their default, allow or deny. Each person
// decides only for themselves.
func (s *Server) chooseAgentApp(w http.ResponseWriter, r *http.Request) {
	member, found := s.crewAgent(r)
	who, person := s.connectingPerson(r)
	if !found || !person {
		http.NotFound(w, r)
		return
	}
	back := "/members/agents/" + member.ID + "?tab=apps"
	entry, offered := s.offeredConnector(r.FormValue("app"))
	if !offered {
		s.turnAway(w, r, back, "that app is not one offered here")
		return
	}
	choice := r.FormValue("choice")
	if _, err := s.crew.ChooseApp(member.ID, who, entry.ID, choice); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	s.note(r, changelog.ConnectorsLetAgent, map[string]string{"agent": member.Name, "connector": entry.Name, "now": choice})
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// setAppForAgents sets the asking person's default for whether their agents may use one app they connected, which every agent they
// left on inherit follows.
func (s *Server) setAppForAgents(w http.ResponseWriter, r *http.Request) {
	who, person := s.connectingPerson(r)
	entry, offered := s.offeredConnector(r.PathValue("id"))
	if !person || !offered {
		http.NotFound(w, r)
		return
	}
	on := r.FormValue("on") == "on"
	if err := s.connectors.SetForAgents(who, entry.ID, on); err != nil {
		s.turnAway(w, r, connectionsPath, err.Error())
		return
	}
	now := "off"
	if on {
		now = "on"
	}
	s.note(r, changelog.ConnectorForAgents, map[string]string{"connector": entry.Name, "now": now})
	http.Redirect(w, r, connectionsPath, http.StatusSeeOther)
}
