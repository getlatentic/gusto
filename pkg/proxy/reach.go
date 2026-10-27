package proxy

import (
	"context"

	"github.com/getlatentic/gusto/pkg/wire"
)

// Reaches says whether an agent that calls in this shape can use the model: its credential answers the shape, which is
// native, or translates it. A model whose credential the proxy does not hold is reached by nothing.
func (p *Proxy) Reaches(ctx context.Context, model string, shape wire.Shape) (native, ok bool) {
	name, _ := Named(model)
	credential, found := p.credentialFor(ctx, Ticket{Credential: name})
	if !found {
		return false, false
	}
	if credential.Answers(shape) {
		return true, true
	}
	return false, credential.Translates(shape)
}
