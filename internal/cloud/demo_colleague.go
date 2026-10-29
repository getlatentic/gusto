package cloud

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/changelog"
)

// colleaguePage is where the person who started a demo asks a colleague in. The colleague is one more person of the demo, and sees
// the demo's room first.
func (s *Server) colleaguePage(w http.ResponseWriter, r *http.Request, person Person) {
	s.drawColleague(w, r, http.StatusOK, person, view{})
}

func (s *Server) drawColleague(w http.ResponseWriter, r *http.Request, status int, person Person, page view) {
	demo, left, found := s.Store.DemoLeft(person.Org)
	if !found {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	page.Title, page.Person, page.IsAdmin = "Bring a colleague", person, person.Role == Admin
	page.Org, page.DemoLeft, page.Book = Org{Slug: person.Org, Name: demo.Company}, left, s.Store.set.Demo.BookURL
	s.render(w, status, "demo-colleague", page)
}

// inviteColleague makes the invitation and shows its link, which works the same sent by mail or pasted in a chat, because a demo is
// short and its visitor has the colleague beside them.
func (s *Server) inviteColleague(w http.ResponseWriter, r *http.Request, person Person) {
	if _, _, found := s.Store.DemoLeft(person.Org); !found {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	address := r.PostFormValue("email")
	secret, err := s.Store.Invite(person.Org, person, address)
	if err != nil {
		s.drawColleague(w, r, refusedStatus(err), person, view{Error: sentence(err), Email: address})
		return
	}
	email, _ := NormalEmail(address)
	s.record(r, person, changelog.MemberInvited, map[string]string{"email": email, "role": string(Runner)})
	s.addToDemoRoom(person, email)
	link := s.link("/invite", secret)
	if s.mails.Allow(email) {
		org, _, _ := s.Store.Org(person.Org)
		s.send(email, person.Email+" invited you to a gusto demo",
			person.Email+" invited you to "+org.Name+"'s gusto demo, where you can work with its agents in a room:\n\n"+link+
				"\n\nThe link works once. The demo ends when its time is up.\n")
	}
	s.drawColleague(w, r, http.StatusOK, person, view{Link: link, Email: email})
}

// entrance is where a signed-in person goes in: a demo's own page, which starts its time when its server is
// ready, and the organization's server for any other.
func (s *Server) entrance(person Person) string {
	if _, _, found := s.Store.DemoLeft(person.Org); found {
		return "/demo/wait"
	}
	return "/open"
}
