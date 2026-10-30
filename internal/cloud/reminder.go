package cloud

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
)

// confirmation is whether the person signed in here has confirmed their address, for the reminder on their
// organization's pages. Jobs is the monthly limit that confirming lifts the trial to.
type confirmation struct {
	Email     string `json:"email"`
	Confirmed bool   `json:"confirmed"`
	Jobs      int    `json:"jobs"`
}

// allowOrgOrigin lets an organization's own address read the answer with the person's cookie, and says which
// organization that is. Any other origin is told nothing.
func (s *Server) allowOrgOrigin(w http.ResponseWriter, r *http.Request) (string, bool) {
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Cache-Control", "no-store")
	origin := r.Header.Get("Origin")
	if !s.isOrgOrigin(origin) {
		return "", false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	parsed, _ := url.Parse(origin)
	return s.orgSlug(parsed.Host)
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
	org, _, _ := s.Store.Org(slug)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(confirmation{Email: person.Email, Confirmed: !person.Trial() || org.Demo != nil, Jobs: s.Store.set.Limits.Verified.Jobs})
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
	if !s.mails.Allow(person.Email) {
		http.Error(w, "that address has been mailed too often", http.StatusTooManyRequests)
		return
	}
	org, _, _ := s.Store.Org(slug)
	s.mailVerification(person, org)
	w.WriteHeader(http.StatusNoContent)
}
