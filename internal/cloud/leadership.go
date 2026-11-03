package cloud

import (
	"context"
	"log/slog"
	"time"
)

// Leadership keeps this front door's claim to the lease, and runs the work only one front door may do while it holds it:
// the controller, the demo sweep and the scaler. Front doors that do not lead serve pages and route from the database.
type Leadership struct {
	Store *Store
	// Holder names this front door, apart from every other.
	Holder string
	// TTL is how long the lease lasts unrenewed, and Every how often it is renewed, well inside TTL.
	TTL, Every time.Duration
}

// Run claims the lease every Every, until ctx ends. While it holds the lease it runs lead, with a context that ends when the
// lease is lost, and with the epoch that fences its writes. It gives the lease up when ctx ends, so another front door takes
// it at once.
func (l *Leadership) Run(ctx context.Context, lead func(ctx context.Context, epoch int64)) {
	held := leading{}
	defer func() {
		held.end()
		if err := l.Store.Resign(l.Holder); err != nil {
			slog.Warn("the lease could not be given up, so it passes when it runs out", "error", err)
		}
	}()
	tick := time.NewTicker(l.Every)
	defer tick.Stop()
	for {
		l.claim(ctx, &held, lead)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (l *Leadership) claim(ctx context.Context, held *leading, lead func(context.Context, int64)) {
	epoch, err := l.Store.Lead(l.Holder, l.TTL)
	switch {
	case err != nil:
		// The lease may have run out unrenewed, and another front door may lead now. Its writes would be refused anyway.
		if held.epoch != 0 && time.Since(held.renewed) > l.TTL {
			slog.Warn("the lease could not be renewed in time, so this front door stops leading", "error", err)
			held.end()
		}
	case epoch == 0:
		if held.epoch != 0 {
			slog.Warn("another front door leads now")
		}
		held.end()
	case epoch != held.epoch:
		held.end()
		slog.Info("this front door leads", "epoch", epoch)
		held.begin(ctx, epoch, lead)
	default:
		held.renewed = time.Now()
	}
}

// leading is the work this front door runs while it leads.
type leading struct {
	epoch   int64
	renewed time.Time
	stop    context.CancelFunc
	done    chan struct{}
}

func (h *leading) begin(ctx context.Context, epoch int64, lead func(context.Context, int64)) {
	inner, stop := context.WithCancel(ctx)
	h.epoch, h.renewed, h.stop, h.done = epoch, time.Now(), stop, make(chan struct{})
	go func(done chan struct{}) {
		defer close(done)
		lead(inner, epoch)
	}(h.done)
}

// end stops the work and waits for it.
func (h *leading) end() {
	if h.stop == nil {
		return
	}
	h.stop()
	<-h.done
	*h = leading{}
}
