package web

import (
	"net/http"
	"slices"

	"github.com/getlatentic/haas/internal/catalog"
)

// Models is what a deployment offers to think with, by harness.
//
// Not every credential answers every harness: a provider can refuse what one
// harness sends while serving another perfectly well. A harness the deployment
// has said nothing about is offered everything, so this only narrows where
// somebody found out that it had to.
type Models map[string][]string

// Everything is the list a deployment offers to whatever asks, which is what a
// deployment that has found no exceptions has.
func Everything(models []string) Models { return Models{"": models} }

// For is what this harness may be pointed at. A harness with no entry of its
// own is offered what everything is offered.
func (m Models) For(harness string) []string {
	if named, found := m[harness]; found {
		return named
	}
	return m[""]
}

// Serves says whether this harness can be given a model at all. It cannot when the
// deployment offers models and none of them is one this harness can use: it has an entry
// of its own, and the entry is empty. A deployment that names no model at all leaves
// the harness to its own default, and serves it.
func (m Models) Serves(harness string) bool {
	named, found := m[harness]
	return !found || len(named) > 0 || len(m[""]) == 0
}

// Offers says whether this model is one this harness may be pointed at.
func (m Models) Offers(harness, model string) bool {
	return slices.Contains(m.For(harness), model)
}

// First is what a harness is given when nobody chose, which is what starting
// one from the catalog does. It is empty when the deployment names no model,
// and the harness falls back to its own default.
func (m Models) First(harness string) string {
	if offered := m.For(harness); len(offered) > 0 {
		return offered[0]
	}
	return ""
}

// unservedMarked is the catalog with each harness that no model here serves made not ready
// and told why, so nothing that starts an agent offers it.
func unservedMarked(offered []catalog.Entry, models Models) []catalog.Entry {
	marked := make([]catalog.Entry, len(offered))
	for i, entry := range offered {
		if entry.Ready && !models.Serves(entry.Name) {
			entry.Ready, entry.Why = false, "no model this deployment offers works with it"
		}
		marked[i] = entry
	}
	return marked
}

// offered is what this deployment offers to think with now. It changes when an organisation's admin changes the
// registry, so what a page says is read when the page is drawn.
func (s *Server) offered() Models {
	if s.live != nil {
		return s.live()
	}
	return s.models
}

// entries is the catalog as it stands: each harness that no model offered now serves is not ready, and says why.
func (s *Server) entries() []catalog.Entry { return unservedMarked(s.catalog, s.offered()) }

// offeredTo is what the person who made this request may think with: what is offered, less what the organisation
// has limited them away from. A person limited to models that are no longer offered is offered none, which is
// different from a deployment that names no model and leaves a harness to its own default.
func (s *Server) offeredTo(r *http.Request) Models {
	offered := s.offered()
	if s.registry == nil {
		return offered
	}
	allowed, limited := s.registry.AllowedFor(s.keeper.who(r))
	if !limited {
		return offered
	}
	narrowed := Models{}
	for name, models := range offered {
		narrowed[name] = slices.DeleteFunc(slices.Clone(models), func(model string) bool { return !slices.Contains(allowed, model) })
	}
	for _, entry := range s.catalog {
		if _, found := narrowed[entry.Name]; !found {
			narrowed[entry.Name] = slices.DeleteFunc(slices.Clone(offered.For(entry.Name)), func(model string) bool { return !slices.Contains(allowed, model) })
		}
	}
	return narrowed
}

// limitedAway says whether this person is offered no model for a harness that this deployment offers models for.
// A deployment that offers none leaves a harness to its own default, which is another thing.
func (s *Server) limitedAway(r *http.Request, harness string) bool {
	return len(s.offeredTo(r).For(harness)) == 0 && len(s.offered().For(harness)) > 0
}
