// Package spare keeps one blank thing made ahead of time, so the next caller that needs one takes it instead of waiting for it
// to be made. Nothing here knows what the thing is: the owner says how to make one and how to remove one, and a spare is only
// ever handed to one caller.
package spare

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Maker is how a pool makes and removes what it keeps.
type Maker struct {
	// Name is a name not used before, for the next spare.
	Name func() string
	// Make creates the spare called name. It may leave one behind when it fails or is stopped.
	Make func(ctx context.Context, name string) error
	// Remove takes away the spare called name, and one that is not there is removed.
	Remove func(ctx context.Context, name string) error
	// Stale lists what an earlier owner left, which nobody holds: a server that was killed made spares it never removed.
	Stale func(ctx context.Context) ([]string, error)
}

// Timing bounds what the pool waits for.
type Timing struct {
	// MakeWithin is how long making one spare may take before it is given up and removed.
	MakeWithin time.Duration
	// RemoveWithin is how long taking one away may take.
	RemoveWithin time.Duration
	// RetryFirst and RetryMost are the wait after a spare could not be made, which doubles each time up to the most.
	RetryFirst, RetryMost time.Duration
}

// Pool keeps at most one spare, made in the background by one goroutine. That goroutine is the only thing that calls the
// maker, so the pool never has two being made; the lock guards the names the pool holds and nothing else, and is never held
// while the maker runs.
type Pool struct {
	maker  Maker
	timing Timing
	key    string

	mu     sync.Mutex
	ready  string
	strays []string
	closed bool

	wake   chan struct{}
	cancel context.CancelFunc
	done   chan struct{}
}

// Open starts a pool whose spares are for callers that ask with key, and makes the first spare. It stops when ctx ends or
// Close is called, and removes what it holds then.
func Open(ctx context.Context, key string, maker Maker, timing Timing) *Pool {
	ctx, cancel := context.WithCancel(ctx)
	p := &Pool{maker: maker, timing: timing, key: key, wake: make(chan struct{}, 1), cancel: cancel, done: make(chan struct{})}
	go p.run(ctx)
	return p
}

// Take hands over the spare to the one caller that asks first, or says there is none, and does not wait for one to be made.
// A spare is made for a key, and a caller with another gets none: what it needs is not what was made.
func (p *Pool) Take(key string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || key != p.key || p.ready == "" {
		return "", false
	}
	name := p.ready
	p.ready = ""
	p.signal()
	return name, true
}

// Return gives back a spare that was taken and not used. It is kept when the pool has room, and removed when it has not.
func (p *Pool) Return(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed && p.ready == "" {
		p.ready = name
		return
	}
	p.strays = append(p.strays, name)
	p.signal()
}

// Offer is the pool as one caller sees it: the key it asks with is fixed.
type Offer struct {
	Pool *Pool
	Key  string
}

// Take is Pool.Take with the offer's key.
func (o Offer) Take() (string, bool) { return o.Pool.Take(o.Key) }

// Return is Pool.Return.
func (o Offer) Return(name string) { o.Pool.Return(name) }

// Close stops the pool and removes the spare it holds and the one it was making, and returns when that is done or after
// wait, whichever is first.
func (p *Pool) Close(wait time.Duration) {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(wait):
		slog.Warn("a spare computer was still being removed when the server stopped")
	}
}

func (p *Pool) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *Pool) run(ctx context.Context) {
	defer close(p.done)
	defer p.drain()
	p.sweep(ctx)
	delay := p.timing.RetryFirst
	for ctx.Err() == nil {
		p.removeStrays()
		if p.holds() {
			if !p.untilWoken(ctx) {
				return
			}
			continue
		}
		if err := p.makeOne(ctx); err != nil {
			slog.Warn("a spare computer could not be made", "error", err, "retry_in", delay)
			if !p.after(ctx, delay) {
				return
			}
			delay = min(2*delay, p.timing.RetryMost)
			continue
		}
		delay = p.timing.RetryFirst
	}
}

// untilWoken waits until a spare is taken or returned, and says whether the pool is still open.
func (p *Pool) untilWoken(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-p.wake:
		return true
	}
}

// after waits for d, and says whether the pool is still open.
func (p *Pool) after(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (p *Pool) holds() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ready != ""
}

// makeOne makes a spare and keeps it. One that fails, hangs past its time or is finished after the pool closed is removed
// by name, since the gateway may have made it though the command that asked did not come back.
func (p *Pool) makeOne(ctx context.Context) error {
	name := p.maker.Name()
	making, cancel := context.WithTimeout(ctx, p.timing.MakeWithin)
	err := p.maker.Make(making, name)
	cancel()
	if err != nil {
		p.remove(name)
		return err
	}
	p.mu.Lock()
	// A spare given back while this one was being made already fills the place.
	surplus := p.closed || p.ready != ""
	if !surplus {
		p.ready = name
	}
	p.mu.Unlock()
	if surplus {
		p.remove(name)
	}
	return ctx.Err()
}

func (p *Pool) sweep(ctx context.Context) {
	if p.maker.Stale == nil {
		return
	}
	stale, err := p.maker.Stale(ctx)
	if err != nil {
		slog.Warn("the spare computers an earlier server left could not be listed", "error", err)
		return
	}
	for _, name := range stale {
		p.remove(name)
	}
}

func (p *Pool) removeStrays() {
	p.mu.Lock()
	strays := p.strays
	p.strays = nil
	p.mu.Unlock()
	for _, name := range strays {
		p.remove(name)
	}
}

// drain removes what the pool still holds when it stops.
func (p *Pool) drain() {
	p.mu.Lock()
	p.closed = true
	held := append(p.strays, p.ready)
	p.strays, p.ready = nil, ""
	p.mu.Unlock()
	for _, name := range held {
		if name != "" {
			p.remove(name)
		}
	}
}

// remove runs on a clock of its own: it is how a stopping pool cleans up, when its context has already ended.
func (p *Pool) remove(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timing.RemoveWithin)
	defer cancel()
	if err := p.maker.Remove(ctx, name); err != nil {
		slog.Warn("a spare computer could not be removed", "sandbox", name, "error", err)
	}
}
