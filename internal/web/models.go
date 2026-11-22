package web

import (
	"net/http"
	"slices"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/pkg/wire"
)

// Models is what a deployment offers to think with. Every engine is offered every model: which pairs work well is for
// the person to find out, and no list kept here could keep up with the engines and models there are.
type Models []string

// Offers says whether this model is one an agent may be pointed at.
func (m Models) Offers(model string) bool { return slices.Contains(m, model) }

// engines is the catalog as it stands now.
func (s *Server) engines() []catalog.Entry {
	if s.liveCatalog != nil {
		return s.liveCatalog()
	}
	return s.catalog
}

// UseLiveCatalog has the catalog read again each time a page asks for it.
func (s *Server) UseLiveCatalog(read func() []catalog.Entry) { s.liveCatalog = read }

// offered is what this deployment offers to think with now. It changes when an organization's admin changes the
// registry, so what a page says is read when the page is drawn.
func (s *Server) offered() Models {
	if s.live != nil {
		return s.live()
	}
	return s.models
}

// offeredTo is what the person who made this request may think with: what is offered, less what the organization
// has limited them away from. A person limited to models that are no longer offered is offered none, which is
// different from a deployment that names no model and leaves an engine to its own default.
func (s *Server) offeredTo(r *http.Request) Models {
	offered := s.offered()
	if s.registry == nil {
		return offered
	}
	allowed, limited := s.registry.AllowedFor(s.askerName(r))
	if !limited {
		return offered
	}
	return slices.DeleteFunc(slices.Clone(offered), func(model string) bool { return !slices.Contains(allowed, model) })
}

// limitedAway says whether this person is offered no model where this deployment offers some. A deployment that
// offers none leaves an engine to its own default, which is another thing.
func (s *Server) limitedAway(r *http.Request) bool {
	return len(s.offeredTo(r)) == 0 && len(s.offered()) > 0
}

// limitedWords is what a person limited away from every model offered is told.
const limitedWords = "you may not use any of the models offered here"

// UseReach has the pickers offer an agent only the models its calls can reach, from what the proxy holds.
func (s *Server) UseReach(reaches func(model string, shape wire.Shape) (native, ok bool)) {
	s.reaches = reaches
}
