package proxy

import (
	"sync/atomic"

	"github.com/getlatentic/gusto/internal/spend"
)

// Counts is what the proxy has done since it started, for the metrics. Errors
// are the calls the model's side failed: no answer at all, or a 5xx. A refusal
// is a cap stopping a call, which is the proxy working and not failing, and a 4xx
// the model returned is the caller's own request.
type Counts struct {
	Calls, Errors, Refused int64
}

type counters struct{ calls, errors, refused atomic.Int64 }

func (c *counters) add(call spend.Call) {
	c.calls.Add(1)
	switch {
	case call.Refused != nil:
		c.refused.Add(1)
	case call.Error != "" || call.Status >= 500:
		c.errors.Add(1)
	}
}

// Counts is the totals so far.
func (p *Proxy) Counts() Counts {
	return Counts{Calls: p.counts.calls.Load(), Errors: p.counts.errors.Load(), Refused: p.counts.refused.Load()}
}
