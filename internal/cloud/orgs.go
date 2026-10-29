package cloud

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
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

// switchable is one of a person's organizations as the switcher on an organization's pages lists it.
type switchable struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Open string `json:"open"`
}

// orgsJSON is the organizations of the person signed in here, for the switcher on their organization's pages. Only
// an organization's own address may read it with the person's cookie: an app's address, or anywhere else, is not
// told it may.
func (s *Server) orgsJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Cache-Control", "no-store")
	if origin := r.Header.Get("Origin"); s.isOrgOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	all, ok := s.memberships(r)
	if !ok {
		http.Error(w, "not signed in", http.StatusUnauthorized)
		return
	}
	orgs, err := s.Store.OrgsOf(all[0].Email)
	if err != nil {
		log.Printf("cloud: a person's organizations could not be read: %v", err)
		http.Error(w, "the organizations could not be read", http.StatusInternalServerError)
		return
	}
	list := make([]switchable, 0, len(orgs))
	for _, org := range orgs {
		open := s.URL.JoinPath("/open").String() + "?" + url.Values{"org": {org.Slug}}.Encode()
		list = append(list, switchable{Name: org.Name, Slug: org.Slug, Open: open})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"orgs": list})
}

// isOrgOrigin says an origin is an organization's address on this front door, by the scheme the front door is on.
func (s *Server) isOrgOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != s.URL.Scheme || parsed.Path != "" {
		return false
	}
	_, ok := s.orgSlug(parsed.Host)
	return ok && parsed.Port() == s.URL.Port()
}
