package cloud

import (
	"encoding/json"
	"net/http"
	"slices"
)

// confirmation is whether the person signed in here has confirmed their address, for the reminder on their
// organization's pages. Tasks is the monthly limit that confirming lifts the trial to.
type confirmation struct {
	Email     string `json:"email"`
	Confirmed bool   `json:"confirmed"`
	Tasks     int    `json:"jobs"`
}

// allowOrgOrigin lets an organization's own pages read the answer with the person's cookie, and says which organization
// that is. Any other page is told nothing. An organization with a host name of its own asks from its origin, which is let
// read the answer. One at a path shares gusto cloud's origin, so the browser lets only gusto's own pages read the answer,
// and the page names its organization (`org`, as cloudLink.page writes it); whether the person is in it is the caller's to
// check.
func (s *Server) allowOrgOrigin(w http.ResponseWriter, r *http.Request) (string, bool) {
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Cache-Control", "no-store")
	if s.OrgPaths {
		slug := r.URL.Query().Get("org")
		return slug, sameSite(r) && ValidSlug(slug) == nil
	}
	origin := r.Header.Get("Origin")
	slug := s.orgOf(origin)
	if slug == "" {
		return "", false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	return slug, true
}

// memberOf is the person signed in here as they are in the organization an origin is, when they are in it.
func (s *Server) memberOf(r *http.Request, slug string) (Person, bool) {
	all, ok := s.memberships(r)
	if !ok {
		return Person{}, false
	}
	at := slices.IndexFunc(all, func(p Person) bool { return p.Org == slug })
	if at < 0 {
		return Person{}, false
	}
	return all[at], true
}

// accountJSON says whether the person has confirmed their address. A demo's visitor has nothing to confirm.
func (s *Server) accountJSON(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.allowOrgOrigin(w, r)
	if !ok {
		http.Error(w, "only an organization's own pages may ask", http.StatusForbidden)
		return
	}
	person, ok := s.memberOf(r, slug)
	if !ok {
		http.Error(w, "not signed in", http.StatusUnauthorized)
		return
	}
	org, _, _ := s.Store.Org(r.Context(), slug)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(confirmation{Email: person.Email, Confirmed: !person.Trial() || org.Demo != nil, Tasks: s.Store.set.Limits.Verified.Tasks})
}

// confirmAgain mails the person the link that confirms their address again, up to the limit for the address.
func (s *Server) confirmAgain(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.allowOrgOrigin(w, r)
	if !ok {
		http.Error(w, "only an organization's own pages may ask", http.StatusForbidden)
		return
	}
	person, ok := s.memberOf(r, slug)
	if !ok {
		http.Error(w, "not signed in", http.StatusUnauthorized)
		return
	}
	if !person.Trial() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.mails.Allow(r.Context(), person.Email) {
		http.Error(w, "that address has been mailed too often", http.StatusTooManyRequests)
		return
	}
	org, _, _ := s.Store.Org(r.Context(), slug)
	s.mailVerification(r.Context(), person, org)
	w.WriteHeader(http.StatusNoContent)
}
