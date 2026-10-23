package proxy

import (
	"context"
	"sync"

	"github.com/getlatentic/gusto/pkg/spend"
)

type resultKey struct{}

// Result is the call a request made, once the proxy has answered it: what it was, what the
// provider said it used and what that cost. A host that passes a request through the proxy
// on behalf of someone with their own account settles that account from it.
type Result struct {
	mu   sync.Mutex
	call spend.Call
	made bool
}

// WithResult returns a context that makes the proxy tell the Result about the model call of
// any request carrying it. A request that is refused before it reaches the model is told too,
// with the refusal on the call.
func WithResult(ctx context.Context) (context.Context, *Result) {
	result := &Result{}
	return context.WithValue(ctx, resultKey{}, result), result
}

// Call is the call the request made, or false when the proxy did not make one.
func (r *Result) Call() (spend.Call, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.call, r.made
}

// report tells the Result of a request's context, if it has one.
func report(ctx context.Context, call spend.Call) {
	result, ok := ctx.Value(resultKey{}).(*Result)
	if !ok {
		return
	}
	result.mu.Lock()
	result.call, result.made = call, true
	result.mu.Unlock()
}
