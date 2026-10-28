package cloud

import (
	"log"
	"net/http"
)

// orgsPage is where a person in several organizations chooses which one to be in. Somebody in one goes straight
// to it, since there is nothing to choose.
func (s *Server) orgsPage(w http.ResponseWriter, r *http.Request) {
	all, ok := s.memberships(r)
	if !ok {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return
	}
	if len(all) == 1 {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	orgs, err := s.Store.OrgsOf(all[0].Email)
	if err != nil {
		log.Printf("cloud: a person's organizations could not be read: %v", err)
		s.render(w, http.StatusInternalServerError, "signin", view{Title: "Sign in", Error: "Something went wrong on our side. Try again."})
		return
	}
	s.render(w, http.StatusOK, "orgs", view{Title: "Choose an organization", Email: all[0].Email, Orgs: orgs, Address: s.Domain})
}
