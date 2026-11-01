package web

import (
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/job"
)

// roomApps is what a person in a room sees above its message box: the apps they connected, and whether its agents may use them.
type roomApps struct {
	Names string
	Let   bool
}

// myRoomApps is the apps the person asking connected that are on, as a room offers them, or nil when they have none to offer.
func (s *Server) myRoomApps(r *http.Request, held instance.Instance) *roomApps {
	who, person := s.connectingPerson(r)
	if !held.Room || !person || !s.hasConnectors() || held.RoleOf(who) == "" {
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
	if len(names) == 0 {
		return nil
	}
	return &roomApps{Names: strings.Join(names, ", "), Let: held.LetsConnections(who)}
}

// letConnections lets a room's agents use the asking member's connected apps for what they ask, or stops them. Each member
// decides only for themselves.
func (s *Server) letConnections(w http.ResponseWriter, r *http.Request) {
	held, found := s.sharedOr404(w, r)
	if !found {
		return
	}
	who, person := s.connectingPerson(r)
	if !held.Room || !person || held.RoleOf(who) == "" {
		http.NotFound(w, r)
		return
	}
	let := r.FormValue("let") == "on"
	if err := s.instances.LetConnections(held.ID, who, let); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := "off"
	if let {
		now = "on"
	}
	s.note(r, changelog.ConnectorsLetInRoom, map[string]string{"room": held.Title, "now": now})
	back := r.FormValue("back")
	if !strings.HasPrefix(back, "/instances/"+held.ID+"/") {
		back = "/instances/" + held.ID
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// usedApps is what a room turn says about the apps it used: whose, and which, so everyone in the room can see it.
func (s *Server) usedApps(record jobs.Record, dir job.Dir) string {
	if !record.InRoom() || !s.hasConnectors() {
		return ""
	}
	spec, err := job.Read(dir)
	if err != nil || spec.Setup == nil || len(spec.Setup.Connectors) == 0 {
		return ""
	}
	var names []string
	for _, id := range spec.Setup.Connectors {
		if entry, known := s.connectors.App(id); known {
			names = append(names, entry.Name)
		}
	}
	return "with " + nameFromAddress(record.Asked) + "'s " + strings.Join(names, ", ")
}
