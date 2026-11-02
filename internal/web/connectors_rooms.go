package web

import (
	"strings"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/job"
)

// usedApps is whose connected apps a room turn could use, and which, for the turn's steps to say.
func (s *Server) usedApps(record jobs.Record, dir job.Dir) string {
	if !record.InRoom() || !s.hasConnectors() {
		return ""
	}
	spec, err := job.Read(dir)
	if err != nil || spec.Setup == nil || len(spec.Setup.Connectors) == 0 {
		return ""
	}
	return nameFromAddress(record.Asked) + "'s " + strings.Join(s.appNames(spec.Setup.Connectors), ", ")
}

// roomAppsLine tells a room's agent which connected apps it has for this message. It keeps one conversation across the room, so
// without this it would look for apps an earlier asker gave it, which this asker's turn does not have.
func (s *Server) roomAppsLine(record jobs.Record, spec job.Spec) string {
	if !record.InRoom() || !s.hasConnectors() {
		return ""
	}
	asker := nameFromAddress(record.Asked)
	if spec.Setup != nil && len(spec.Setup.Connectors) > 0 {
		return "\nFor this message you have " + asker + "'s connected apps: " + strings.Join(s.appNames(spec.Setup.Connectors), ", ") + ". Use them only for what " + asker + " asks.\n"
	}
	head := "\nFor this message you have no connected apps. "
	if names := s.connectedApps(record.Asked); len(names) > 0 && !s.connectionsAllowed(record) {
		return head + asker + " has not let you use their " + strings.Join(names, ", ") + ". If the work needs one, say so in one sentence: " +
			asker + " can let you use them on your Apps page. Nothing is disconnected, so never suggest reconnecting.\n"
	}
	return head + "A room's agents get a person's apps only for what that person asks, and " + asker +
		" asked this. Apps you used earlier for someone else are not available now, so do not look for them. If the work needs one, say so in" +
		" one sentence: " + asker + "'s request does not carry another person's apps, and the person whose app it is can ask for it themselves." +
		" Nothing is disconnected, so never suggest reconnecting.\n"
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
