package cloud

import (
	"net/http"
	"slices"

	"github.com/getlatentic/gusto/internal/changelog"
)

// teamRow is a team as the organization's page shows it: who is in it, and who of the organization could be added.
type teamRow struct {
	Team
	People []spacePerson
	Others []string
}

// spacePerson is a person in a team, who may be taken out of it when they are in another too.
type spacePerson struct {
	Email     string
	Removable bool
}

func teamRows(spaces []Team, members []Person) []teamRow {
	in := map[string]int{}
	for _, space := range spaces {
		for _, email := range space.Members {
			in[email]++
		}
	}
	rows := make([]teamRow, 0, len(spaces))
	for _, space := range spaces {
		row := teamRow{Team: space}
		for _, email := range space.Members {
			row.People = append(row.People, spacePerson{Email: email, Removable: in[email] > 1})
		}
		for _, member := range members {
			if !slices.Contains(space.Members, member.Email) {
				row.Others = append(row.Others, member.Email)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// The forms on the organization's page that an admin changes its teams with. Each change goes in the organization's
// change record, and its server reads who is in which team from the list of people it is sent.

// peopleRoutes are the forms an admin changes the organization's people and teams with.
func (s *Server) peopleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /org/invite", s.guard(s.invite))
	mux.HandleFunc("POST /org/invites/cancel", s.guard(s.cancelInvitation))
	mux.HandleFunc("POST /org/people/remove", s.guard(s.removePerson))
	mux.HandleFunc("POST /org/people/role", s.guard(s.changeRole))
	mux.HandleFunc("POST /org/teams", s.guard(s.makeTeam))
	mux.HandleFunc("POST /org/teams/rename", s.guard(s.renameTeam))
	mux.HandleFunc("POST /org/teams/add", s.guard(s.addToTeam))
	mux.HandleFunc("POST /org/teams/remove", s.guard(s.removeFromTeam))
}

func (s *Server) makeTeam(w http.ResponseWriter, r *http.Request, person Person) {
	made, err := s.Store.MakeTeam(r.Context(), person.Org, person, r.PostFormValue("name"))
	if err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	s.record(r, person, changelog.TeamMade, map[string]string{"workspace": made.ID, "name": made.Name})
	http.Redirect(w, r, "/org#teams", http.StatusSeeOther)
}

func (s *Server) renameTeam(w http.ResponseWriter, r *http.Request, person Person) {
	id, name := r.PostFormValue("team"), r.PostFormValue("name")
	if err := s.Store.RenameTeam(r.Context(), person.Org, person, id, name); err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	name, _ = teamName(name)
	s.record(r, person, changelog.TeamRenamed, map[string]string{"workspace": id, "name": name})
	http.Redirect(w, r, "/org#teams", http.StatusSeeOther)
}

func (s *Server) addToTeam(w http.ResponseWriter, r *http.Request, person Person) {
	id := r.PostFormValue("team")
	email, _ := NormalEmail(r.PostFormValue("email"))
	if err := s.Store.AddToTeam(r.Context(), person.Org, person, id, email); err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	s.record(r, person, changelog.TeamAdded, map[string]string{"workspace": id, "email": email})
	http.Redirect(w, r, "/org#teams", http.StatusSeeOther)
}

// removeFromTeam takes a person out of a team. Their computer in it is marked, and the organization's server
// removes it; what they keep in other teams stays.
func (s *Server) removeFromTeam(w http.ResponseWriter, r *http.Request, person Person) {
	id := r.PostFormValue("team")
	email, _ := NormalEmail(r.PostFormValue("email"))
	if err := s.Store.RemoveFromTeam(r.Context(), person.Org, person, id, email); err != nil {
		s.drawOrg(r.Context(), w, refusedStatus(err), person, sentence(err))
		return
	}
	s.record(r, person, changelog.TeamLeft, map[string]string{"workspace": id, "email": email})
	http.Redirect(w, r, "/org#teams", http.StatusSeeOther)
}
