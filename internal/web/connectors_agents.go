package web

import (
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/changelog"
)

// agentApps is what an agent's Apps tab shows the person looking: the apps they connected, and whether the agent may use them.
type agentApps struct {
	Names string
	Let   bool
	// Person is false for the shared sign-in, which is nobody and has no apps to give.
	Person bool
}

// connectedApps is the name of each app a person connected that an admin has turned on.
func (s *Server) connectedApps(who string) []string {
	if !s.hasConnectors() || who == "" {
		return nil
	}
	views, err := s.connectorViews(who)
	if err != nil {
		return nil
	}
	var names []string
	for _, view := range views {
		if view.On && view.Usable && view.Connected {
			names = append(names, view.Name)
		}
	}
	return names
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
	shown.Names = strings.Join(s.connectedApps(who), ", ")
	if held, found := s.crew.Get(member); found {
		shown.Let = held.UsesAppsOf(who)
	}
	return shown
}

// letAgentApps lets an agent use the asking person's connected apps for what they ask it, or stops it. Each person decides only
// for themselves.
func (s *Server) letAgentApps(w http.ResponseWriter, r *http.Request) {
	member, found := s.crewAgent(r)
	who, person := s.connectingPerson(r)
	if !found || !person {
		http.NotFound(w, r)
		return
	}
	let := r.FormValue("let") == "on"
	back := "/members/agents/" + member.ID + "?tab=apps"
	if _, err := s.crew.LetApps(member.ID, who, let); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	now := "off"
	if let {
		now = "on"
	}
	s.note(r, changelog.ConnectorsLetAgent, map[string]string{"agent": member.Name, "now": now})
	http.Redirect(w, r, back, http.StatusSeeOther)
}
