package cloud

import (
	"log"
	"net/http"
)

// person is who a request is signed in as, if their account is still there.
func (s *Server) person(r *http.Request) (Person, bool) {
	email, ok := s.Sessions.Who(r)
	if !ok {
		return Person{}, false
	}
	person, found, err := s.Store.Person(email)
	if err != nil || !found {
		return Person{}, false
	}
	return person, true
}

func (s *Server) guard(next func(http.ResponseWriter, *http.Request, Person)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		person, ok := s.person(r)
		if !ok {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}
		next(w, r, person)
	}
}

// org is the organisation a person belongs to, and only that one: the page
// reads the person's own record to find it, and takes no name from the request.
func (s *Server) org(w http.ResponseWriter, r *http.Request, person Person) {
	notice := ""
	if r.URL.Query().Get("sent") != "" {
		notice = "The link is on its way."
	}
	s.drawOrg(w, http.StatusOK, person, notice)
}

func (s *Server) drawOrg(w http.ResponseWriter, status int, person Person, notice string) {
	org, found, err := s.Store.Org(person.Org)
	if err != nil || !found {
		log.Printf("cloud: an organisation is missing its record: %v", err)
		s.render(w, http.StatusInternalServerError, "signin", view{Title: "Sign in", Error: "Something went wrong on our side. Try again."})
		return
	}
	quota, err := s.Store.Quota(org.Slug)
	if err != nil {
		log.Printf("cloud: an organisation is missing its quota: %v", err)
	}
	computers := 0
	if registry, err := s.Store.Computers(org.Slug); err == nil {
		if all, err := registry.All(); err == nil {
			computers = len(all)
		}
	}
	server, canOpen := "", false
	if s.Controller != nil {
		_, state := s.Controller.Route(org.Slug)
		if state == "" {
			state = Provisioning
		}
		server, canOpen = string(state), state == Ready
	}
	members, _ := s.Store.Members(org.Slug)
	invited, _ := s.Store.Invitations(org.Slug)
	v := view{Members: members, Invited: invited, IsAdmin: person.Role == Admin,
		CanInvite: person.Role == Admin && quota.Invites && len(members)+len(invited) < quota.Members, Server: server, CanOpen: canOpen, Refresh: server != "" && !canOpen, Computers: computers, Domain: org.Domain, Title: org.Name, Org: org, Person: person, Address: org.Slug + "." + s.Domain, Quota: quota,
		Trial: s.Store.set.Limits.Trial, Verified: s.Store.set.Limits.Verified, Notice: notice}
	s.render(w, status, "org", v)
}

// resend mails the verification link again, up to the limit for the address.
func (s *Server) resend(w http.ResponseWriter, r *http.Request, person Person) {
	if !person.Trial() {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	if !s.mails.Allow(person.Email) {
		s.drawOrg(w, http.StatusTooManyRequests, person, "That address has been mailed too often. Try again later.")
		return
	}
	org, _, _ := s.Store.Org(person.Org)
	s.mailVerification(person, org)
	http.Redirect(w, r, "/org?sent=1", http.StatusSeeOther)
}

func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	s.Sessions.Clear(w)
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}
