package web

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// On a person's own computer gusto installs an agent it does not find, at the version the runner image carries, into its own
// folder, so the person does not have to run npm themselves.

// agentInstaller installs agents on this computer.
type agentInstaller interface {
	Installable(name string) bool
	Install(ctx context.Context, name string) (string, error)
}

// UseInstaller is what installs agents on this computer, for gusto on a person's own one.
func (s *Server) UseInstaller(installer agentInstaller) { s.installer = installer }

// installing is how long an install may take: npm fetches the agent and everything it needs.
const installing = 10 * time.Minute

// installable says whether the person asking can have gusto install an agent here: gusto is on their own computer, they run it,
// and the agent is one gusto knows how to install.
func (s *Server) installable(name string) bool {
	return s.installer != nil && s.local != nil && s.installer.Installable(name)
}

// handInstall says whether an agent this computer lacks is one the person installs themselves, with the command shown for it.
func (s *Server) handInstall() bool {
	for _, entry := range s.missingEngines() {
		if !s.installable(entry.Name) {
			return true
		}
	}
	return false
}

func (s *Server) installAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	back := r.FormValue("back")
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/members?new=1"
	}
	if !s.admin(r) || !s.installable(name) {
		s.turnAway(w, r, back, "gusto installs agents only on a person's own computer, for the person who runs it")
		return
	}
	// The install goes on if the person leaves the page, so it is not left half done.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), installing)
	defer cancel()
	if _, err := s.installer.Install(ctx, name); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
