package web

import (
	"cmp"
	"net/http"
)

// personShell is what each of a person's own pages draws around itself: the side list, who is signed in, and, behind gusto
// cloud, the organization and the way to its others.
type personShell struct {
	Rail        railList
	SignedAs    signedAs
	Admin       bool
	Connections bool
	Org         *orgLinks
	Workspace   string
	Workspaces  *shownWorkspace
}

// orgLinks is the organization a server is for, behind gusto cloud, and where its pages and the person's account are.
type orgLinks struct {
	Slug, Name                string
	Manage, OrgsFrom          string
	AccountFrom, ConfirmAgain string
}

// personShellJSON answers the shell for whoever is signed in.
func (s *Server) personShellJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s.asJSON(w, s.personShellOf(r))
}

func (s *Server) personShellOf(r *http.Request) personShell {
	shell := personShell{
		Rail: s.railFor(r, ""), SignedAs: s.signedAs(r), Admin: s.admin(r), Connections: s.hasConnectors(),
		Workspace: s.workspaceOf(r), Workspaces: s.shownWorkspaces(r),
	}
	if s.front != nil && s.front.Org != "" {
		shell.Org = &orgLinks{
			Slug: s.front.Org, Name: cmp.Or(s.front.Name, s.front.Org), Manage: s.front.URL + "/orgs",
			OrgsFrom: s.front.URL + "/orgs.json", AccountFrom: s.front.URL + "/account.json", ConfirmAgain: s.front.URL + "/account/confirm",
		}
	}
	return shell
}
