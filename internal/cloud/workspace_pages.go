package cloud

import (
	"net/http"
	"slices"

	"github.com/getlatentic/gusto/internal/changelog"
)

// workspaceRow is a workspace as the organization's page shows it: who is in it, and who of the organization could be added.
type workspaceRow struct {
	Workspace
	Others []string
}

func workspaceRows(spaces []Workspace, members []Person) []workspaceRow {
	rows := make([]workspaceRow, 0, len(spaces))
	for _, space := range spaces {
		row := workspaceRow{Workspace: space}
		for _, member := range members {
			if !slices.Contains(space.Members, member.Email) {
				row.Others = append(row.Others, member.Email)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// The forms on the organization's page that an admin changes its workspaces with. Each change goes in the organization's
// change record, and its server reads who is in which workspace from the list of people it is sent.

func (s *Server) makeWorkspace(w http.ResponseWriter, r *http.Request, person Person) {
	made, err := s.Store.MakeWorkspace(r.Context(), person.Org, person, r.PostFormValue("name"))
	if err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	s.record(r, person, changelog.WorkspaceMade, map[string]string{"workspace": made.ID, "name": made.Name})
	http.Redirect(w, r, "/org#workspaces", http.StatusSeeOther)
}

func (s *Server) renameWorkspace(w http.ResponseWriter, r *http.Request, person Person) {
	id, name := r.PostFormValue("workspace"), r.PostFormValue("name")
	if err := s.Store.RenameWorkspace(r.Context(), person.Org, person, id, name); err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	name, _ = workspaceName(name)
	s.record(r, person, changelog.WorkspaceRenamed, map[string]string{"workspace": id, "name": name})
	http.Redirect(w, r, "/org#workspaces", http.StatusSeeOther)
}

func (s *Server) addToWorkspace(w http.ResponseWriter, r *http.Request, person Person) {
	id := r.PostFormValue("workspace")
	email, _ := NormalEmail(r.PostFormValue("email"))
	if err := s.Store.AddToWorkspace(r.Context(), person.Org, person, id, email); err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	s.record(r, person, changelog.WorkspaceAdded, map[string]string{"workspace": id, "email": email})
	http.Redirect(w, r, "/org#workspaces", http.StatusSeeOther)
}

// removeFromWorkspace takes a person out of a workspace. Their computer in it is marked, and the organization's server
// removes it; what they keep in other workspaces stays.
func (s *Server) removeFromWorkspace(w http.ResponseWriter, r *http.Request, person Person) {
	id := r.PostFormValue("workspace")
	email, _ := NormalEmail(r.PostFormValue("email"))
	if err := s.Store.RemoveFromWorkspace(r.Context(), person.Org, person, id, email); err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	s.record(r, person, changelog.WorkspaceLeft, map[string]string{"workspace": id, "email": email})
	http.Redirect(w, r, "/org#workspaces", http.StatusSeeOther)
}
