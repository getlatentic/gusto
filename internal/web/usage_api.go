package web

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/getlatentic/gusto/internal/usage"
)

// usageReader reads the query, and answers the request itself when it cannot be read or the index is not here.
// It marks an answer made while the index is still being filled, since a total taken then is not the whole.
func (s *Server) usageReader(w http.ResponseWriter, r *http.Request) (usageQuery, bool) {
	if s.usage.store == nil {
		http.Error(w, "this server keeps no usage index", http.StatusNotFound)
		return usageQuery{}, false
	}
	query, allowed, err := s.readUsageQuery(r)
	switch {
	case !allowed:
		http.NotFound(w, r)
		return query, false
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return query, false
	}
	if building := s.usage.building(); building.Building {
		w.Header().Set("Usage-Index-Building", fmt.Sprintf("%d of %d tasks", building.Done, building.Total))
	}
	return query, true
}

func (s *Server) usageFailed(w http.ResponseWriter, err error) {
	if errors.Is(err, usage.ErrNotFound) {
		http.Error(w, "no such session", http.StatusNotFound)
		return
	}
	http.Error(w, "the usage could not be read: "+err.Error(), http.StatusInternalServerError)
}

type sessionsAnswer struct {
	Sessions []usage.Session `json:"sessions"`
	Next     usage.Cursor    `json:"next,omitempty"`
}

type callsAnswer struct {
	Calls []usage.Row  `json:"calls"`
	Next  usage.Cursor `json:"next,omitempty"`
}

func (s *Server) usageSessionsJSON(w http.ResponseWriter, r *http.Request) {
	query, ok := s.usageReader(w, r)
	if !ok {
		return
	}
	sessions, next, err := s.usage.store.Sessions(r.Context(), query.Filter, query.Page)
	if err != nil {
		s.usageFailed(w, err)
		return
	}
	if sessions == nil {
		sessions = []usage.Session{}
	}
	s.asJSON(w, sessionsAnswer{Sessions: sessions, Next: next})
}

func (s *Server) usageCallsJSON(w http.ResponseWriter, r *http.Request) {
	query, ok := s.usageReader(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("session")
	if id == "" {
		http.Error(w, "session is the session whose calls to list", http.StatusBadRequest)
		return
	}
	rows, next, err := s.sessionCalls(r, id, query)
	if err != nil {
		s.usageFailed(w, err)
		return
	}
	s.asJSON(w, callsAnswer{Calls: append([]usage.Row{}, rows...), Next: next})
}

func (s *Server) usageTotalsJSON(w http.ResponseWriter, r *http.Request) {
	query, ok := s.usageReader(w, r)
	if !ok {
		return
	}
	totals, err := s.usage.store.Totals(r.Context(), query.Filter)
	if err != nil {
		s.usageFailed(w, err)
		return
	}
	s.asJSON(w, totals)
}

func (s *Server) usageFacetsJSON(w http.ResponseWriter, r *http.Request) {
	query, ok := s.usageReader(w, r)
	if !ok {
		return
	}
	facets, err := s.usageFacets(r, query.Filter)
	if err != nil {
		s.usageFailed(w, err)
		return
	}
	s.asJSON(w, nonNil(facets))
}

// nonNil makes every list of the facets a list, so that a client reads [] and not null.
func nonNil(f usage.Facets) usage.Facets {
	for _, list := range []*[]string{&f.People, &f.Teams, &f.Projects, &f.Engines, &f.Agents, &f.Providers, &f.Models} {
		if *list == nil {
			*list = []string{}
		}
	}
	return f
}
