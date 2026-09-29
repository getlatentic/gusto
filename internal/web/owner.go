package web

import (
	"net/http"
	"strings"

	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
)

// sharedSignIn is who the shared password signs somebody in as. It is the
// operator's own sign-in, so it is an admin, and what it made belongs to no
// one person.
const sharedSignIn = "the shared sign-in"

// UseAdmins names the people who see everybody's work as well as their own.
func (s *Server) UseAdmins(emails []string) {
	s.admins = map[string]bool{}
	for _, email := range emails {
		s.admins[strings.ToLower(strings.TrimSpace(email))] = true
	}
}

// roleFor is the role a person the provider vouched for signs in with, or that
// they are refused. An address named in HAAS_ADMINS is an admin whatever their
// groups say, so the operator cannot lock themselves out by mapping groups.
func (s *Server) roleFor(who Person) (Role, bool) {
	if s.admins[strings.ToLower(strings.TrimSpace(who.Email))] {
		return Admin, true
	}
	return s.identity.access.roleOf(who)
}

func (s *Server) admin(r *http.Request) bool { return s.keeper.role(r) == Admin }

// adminOnly lets through only an admin. Changing what agents are given changes
// what every later run may do, so it is not something a person who runs work
// can do; the answer is 403 rather than 404 because the page is theirs to read.
func (s *Server) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(r) {
			http.Error(w, "changing what agents are given is for an administrator", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// mayUse says whether the person asking may see what owner made. Work with no
// owner, and work the shared sign-in made, has no person to belong to, so only
// an admin sees it.
func (s *Server) mayUse(r *http.Request, owner string) bool {
	if s.admin(r) {
		return true
	}
	return owner != "" && owner != sharedSignIn && strings.EqualFold(owner, s.keeper.who(r))
}

// owned is the instance the path names when the person asking may use it.
func (s *Server) owned(r *http.Request) (instance.Instance, bool) {
	held, found := s.instances.Get(r.PathValue("id"))
	if !found || !s.mayUse(r, held.Who) {
		return instance.Instance{}, false
	}
	return held, true
}

// mine is owned that answers 404 itself. Somebody else's agent gets the answer a
// missing one does, so a guess cannot tell the two apart.
func (s *Server) mine(w http.ResponseWriter, r *http.Request) (instance.Instance, bool) {
	held, found := s.owned(r)
	if !found {
		http.NotFound(w, r)
	}
	return held, found
}

func (s *Server) ownInstances(r *http.Request, all []instance.Instance) []instance.Instance {
	var kept []instance.Instance
	for _, each := range all {
		if s.mayUse(r, each.Who) {
			kept = append(kept, each)
		}
	}
	return kept
}

func (s *Server) ownJobs(r *http.Request, all []jobs.Record) []jobs.Record {
	var kept []jobs.Record
	for _, each := range all {
		if s.mayUse(r, each.Asked) {
			kept = append(kept, each)
		}
	}
	return kept
}

func (s *Server) ownLineups(r *http.Request, all []compare.Lineup) []compare.Lineup {
	var kept []compare.Lineup
	for _, each := range all {
		if s.mayUse(r, each.Who) {
			kept = append(kept, each)
		}
	}
	return kept
}
