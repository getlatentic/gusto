package cloud

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/getlatentic/gusto/internal/entry"
)

// memberships is the person a request is signed in as in each organization they are in, and false when nobody is
// signed in or their account is gone.
func (s *Server) memberships(r *http.Request) ([]Person, bool) {
	email, ok := s.Sessions.Who(r)
	if !ok {
		return nil, false
	}
	all, err := s.Store.Memberships(email)
	if err != nil || len(all) == 0 {
		return nil, false
	}
	return all, true
}

// chosen is the membership a request is for: the organization its address names with ?org=, then the one the
// person chose last, then their only one. A person in several who has chosen none has none.
func (s *Server) chosen(r *http.Request, all []Person) (Person, bool) {
	for _, want := range []string{r.URL.Query().Get("org"), s.Sessions.Chosen(r)} {
		if at := slices.IndexFunc(all, func(p Person) bool { return p.Org == want }); want != "" && at >= 0 {
			return all[at], true
		}
	}
	if len(all) == 1 {
		return all[0], true
	}
	return Person{}, false
}

// person is who a request is signed in as, in the organization it is for.
func (s *Server) person(r *http.Request) (Person, bool) {
	all, ok := s.memberships(r)
	if !ok {
		return Person{}, false
	}
	return s.chosen(r, all)
}

// guard lets a page through for a person signed in to one of their organizations. Somebody signed out goes to sign
// in, and a person in several organizations who has not said which goes to choose one. An organization named with
// ?org= is remembered, so a link to one organization's page switches to it.
func (s *Server) guard(next func(http.ResponseWriter, *http.Request, Person)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, ok := s.memberships(r)
		if !ok {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}
		person, ok := s.chosen(r, all)
		if !ok {
			http.Redirect(w, r, "/orgs", http.StatusSeeOther)
			return
		}
		if person.Org != s.Sessions.Chosen(r) {
			s.Sessions.Choose(w, person.Org)
		}
		next(w, r, person)
	}
}

// org is the organization a person is in. It is one the store says they belong
// to: a name the request gives only chooses among those.
func (s *Server) org(w http.ResponseWriter, r *http.Request, person Person) {
	notice := ""
	if r.URL.Query().Get("sent") != "" {
		notice = "The link is on its way."
	}
	if r.URL.Query().Get("billing") == "started" {
		notice = "Thank you. The plan changes here once Stripe confirms the payment, which takes a few seconds."
	}
	s.drawOrg(w, http.StatusOK, person, notice)
}

func (s *Server) drawOrg(w http.ResponseWriter, status int, person Person, notice string) {
	org, found, err := s.Store.Org(person.Org)
	if err != nil || !found {
		log.Printf("cloud: an organization is missing its record: %v", err)
		s.render(w, http.StatusInternalServerError, "signin", view{Title: "Sign in", Error: "Something went wrong on our side. Try again."})
		return
	}
	quota, err := s.Store.Quota(org.Slug)
	if err != nil {
		log.Printf("cloud: an organization is missing its quota: %v", err)
	}
	computers, byAddress := 0, map[string]string{}
	if registry, err := s.Store.Computers(org.Slug); err == nil {
		if all, err := registry.All(); err == nil {
			computers = len(all)
			for _, each := range all {
				byAddress[each.User] = string(each.State)
			}
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
	admins := 0
	for _, member := range members {
		if member.Role == Admin {
			admins++
		}
	}

	invited, _ := s.Store.Invitations(org.Slug)
	all, _ := s.Store.Memberships(person.Email)
	v := view{ComputerOf: byAddress, Members: members, Admins: admins, Invited: invited, IsAdmin: person.Role == Admin,
		CanInvite: person.Role == Admin && quota.Invites && len(members)+len(invited) < quota.Members, Server: server, CanOpen: canOpen, Refresh: server != "" && !canOpen, Computers: computers, Domain: org.Domain, Title: org.Name, Org: org, Person: person, Address: org.Slug + "." + s.Domain, Quota: quota,
		Trial: s.Store.set.Limits.Trial, Verified: s.Store.set.Limits.Verified, Notice: notice, Several: len(all) > 1, Credit: creditLeft(quota.CreditUSD, s.Store.CreditUsed(org.Slug)),
		Plan: s.planOf(org, person), Console: true}
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

// signedOutThere finishes a sign-out a person began in their organization's server, which has ended its own session
// and vouches, with the key it shares with the front door, that this browser asked. A link without that word is
// sent home and signs nobody out, so another site cannot sign a person out by linking here.
func (s *Server) signedOutThere(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("org")
	if s.Controller == nil || ValidSlug(slug) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	key, err := s.Controller.EntryKey(slug)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, err := entry.VerifyExit(key, slug, r.URL.Query().Get("t"), time.Now()); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.signOut(w, r)
}

// credit is what is left of an organization's free credit on gusto's models, out of how much it was given.
type credit struct{ Left, Of string }

// creditLeft is what is left of an organization's credit, as a person reads it, and nil when it has none. What is left is
// rounded down to the cent, so a credit that has been used never reads as whole.
func creditLeft(given, used float64) *credit {
	if given <= 0 {
		return nil
	}
	return &credit{Left: fmt.Sprintf("$%.2f", math.Floor(max(given-used, 0)*100)/100), Of: fmt.Sprintf("$%.2f", given)}
}
