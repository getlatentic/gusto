package web

import (
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// usedApps is whose connected apps a room turn could use, and which, for the turn's steps to say.
func (s *Server) usedApps(record tasks.Record, dir task.Dir) string {
	if !record.InRoom() || !s.hasConnectors() {
		return ""
	}
	spec, err := task.Read(dir)
	if err != nil || spec.Setup == nil || len(spec.Setup.Connectors) == 0 {
		return ""
	}
	return nameFromAddress(record.Asked) + "'s " + strings.Join(s.appNames(spec.Setup.Connectors), ", ")
}

// roomAppsLine tells a room's agent which connected apps it has for this message, and which of the asker's it was not let use. It
// keeps one conversation across the room, so without this it would look for apps an earlier asker gave it, which this asker's turn
// does not have.
func (s *Server) roomAppsLine(record tasks.Record, spec task.Spec) string {
	if !record.InRoom() || !s.hasConnectors() {
		return ""
	}
	asker := nameFromAddress(record.Asked)
	reconnect := s.reconnectNote(record, asker)
	var given []string
	if spec.Setup != nil {
		given = spec.Setup.Connectors
	}
	withheld := ""
	if names := s.withheldApps(record, given); len(names) > 0 {
		withheld = " " + asker + " has not let you use their " + strings.Join(names, ", ") + "; if the work needs one, say so in one sentence: " +
			asker + " can let you use it on your Apps page." + reconnect
	}
	if len(given) > 0 {
		return "\nFor this message you have " + asker + "'s connected apps: " + strings.Join(s.appNames(given), ", ") + ". Use them only for what " + asker + " asks." + withheld + "\n"
	}
	if withheld != "" {
		return "\nFor this message you have no connected apps." + withheld + "\n"
	}
	return "\nFor this message you have no connected apps. A room's agents get a person's apps only for what that person asks, and " + asker +
		" asked this. Apps you used earlier for someone else are not available now, so do not look for them. If the work needs one, say so in" +
		" one sentence: " + asker + "'s request does not carry another person's apps, and the person whose app it is can ask for it themselves." +
		reconnect + "\n"
}

// reconnectNote tells a room's agent whether anything of the asker's needs connecting again: only a sign-in that no longer
// works does, and the rest is connected.
func (s *Server) reconnectNote(record tasks.Record, asker string) string {
	views, err := s.connectorViews(record.Asked, record.Workspace)
	if err != nil {
		return ""
	}
	var names []string
	for _, view := range views {
		if view.On && view.Usable && view.SignInAgain {
			names = append(names, view.Name)
		}
	}
	if len(names) == 0 {
		return " Nothing is disconnected, so never suggest reconnecting."
	}
	return " " + asker + "'s sign-in to " + strings.Join(names, ", ") + " needs signing in again, so you cannot use it: if the work needs it, say so in one sentence:" +
		" " + asker + " signs in again on their Connections page. Nothing else is disconnected."
}

// withheldApps is the name of each app the asker connected that an admin turned on and that this turn was not given.
func (s *Server) withheldApps(record tasks.Record, given []string) []string {
	views, err := s.connectorViews(record.Asked, record.Workspace)
	if err != nil {
		return nil
	}
	var names []string
	for _, view := range views {
		if view.On && view.Usable && view.Connected && !slices.Contains(given, view.ID) {
			names = append(names, view.Name)
		}
	}
	return names
}

// appNames is the catalog name of each connector given.
func (s *Server) appNames(ids []string) []string {
	var names []string
	for _, id := range ids {
		if entry, known := s.connectors.App(id); known {
			names = append(names, entry.Name)
		}
	}
	return names
}
