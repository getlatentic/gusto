package cloud

import (
	"context"
	"log/slog"
	"time"
)

// Run provisions every organization there is and then keeps their servers running, until the context ends: when this front
// door stops leading, or stops. It leaves the servers running, for the next leader; StopAll stops them. A server that has
// stopped is started again, at first at once and then less often, so one that cannot start does not take the machine with
// it.
func (c *Controller) Run(ctx context.Context, every time.Duration) {
	orgs, err := c.Store.Orgs(ctx)
	if err != nil {
		slog.Error("the organizations could not be listed", "error", err)
	}
	c.resume(ctx, orgs)
	for _, org := range orgs {
		go c.Provision(ctx, org.Slug)
	}
	for {
		// An organization that waits for a node is looked at often, since a visitor waits on it; otherwise every so often.
		next := every
		if c.Waiting() > 0 {
			next = min(every, c.waitEvery())
		}
		timer := time.NewTimer(next)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			c.supervise(ctx)
		}
	}
}

// resume holds what each server was before it is started again, so a request for it is told it is starting and not that it is
// not there. One that was ready stays ready in the database, since a server on a host apart outlives the gusto cloud server
// that started it and provisioning finds it running; any other is written down as starting.
func (c *Controller) resume(ctx context.Context, orgs []Org) {
	for _, org := range orgs {
		runtime, err := c.Store.Runtime(ctx, org.Slug)
		switch {
		case err != nil || runtime.State == "":
		case runtime.State == Ready:
			runtime.State = Starting
			c.put(org.Slug, runtime, true)
		default:
			c.set(ctx, org.Slug, runtime, Starting, "")
		}
	}
}

// survey is what the supervisor does something about, read at one moment.
type survey struct {
	ready   map[string]orgState
	due     []string
	waiting []string
	unsaved []string
}

func (c *Controller) survey() survey {
	seen := survey{ready: map[string]orgState{}}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for slug, held := range c.orgs {
		switch state := held.runtime.State; {
		case state == Ready:
			seen.ready[slug] = held
		case state == Failed && !now.Before(held.retryAt):
			seen.due = append(seen.due, slug)
		case state == Waiting && !now.Before(held.retryAt):
			seen.waiting = append(seen.waiting, slug)
		}
		if held.unsaved {
			seen.unsaved = append(seen.unsaved, slug)
		}
	}
	return seen
}

// supervise restarts what has stopped or failed, retries what waits, writes again what could not be written, forgets the
// restarts of a server that has stayed up, and provisions what was signed up on a gusto cloud server that does not lead. Whether
// a server is alive is asked of its node outside c.mu, so routing does not wait on a node.
func (c *Controller) supervise(ctx context.Context) {
	seen := c.survey()
	due := seen.due
	for slug, held := range seen.ready {
		switch {
		case !c.alive(slug, held.runtime):
			due = append(due, slug)
		case held.runtime.Restarts > 0 && time.Since(held.readyAt) >= c.stableFor():
			c.rewrite(ctx, slug, func(s orgState) bool { return s.runtime.State == Ready && s.runtime.Restarts > 0 },
				func(r *Runtime) { r.Restarts = 0 })
		}
	}
	for _, slug := range due {
		c.restart(ctx, slug)
	}
	for _, slug := range seen.waiting {
		go c.reprovision(ctx, slug)
	}
	for _, slug := range seen.unsaved {
		c.rewrite(ctx, slug, func(s orgState) bool { return s.unsaved }, func(*Runtime) {})
	}
	c.provisionNew(ctx)
}

func (c *Controller) stableFor() time.Duration {
	if c.Stable > 0 {
		return c.Stable
	}
	return 2 * time.Minute
}

// rewrite writes an organization's held runtime again, after edit, when needs says it is still wanted once the organization is
// not being provisioned: one that is will write what it makes anyway.
func (c *Controller) rewrite(ctx context.Context, slug string, needs func(orgState) bool, edit func(*Runtime)) {
	held := c.lock(slug)
	if !held.TryLock() {
		return
	}
	defer held.Unlock()
	state, found := c.stateOf(slug)
	if !found || !needs(state) {
		return
	}
	edit(&state.runtime)
	c.set(ctx, slug, state.runtime, state.runtime.State, state.runtime.Error)
}

// provisionNew provisions each organization that has no runtime yet and that this controller does not hold.
func (c *Controller) provisionNew(ctx context.Context) {
	slugs, err := c.Store.Unprovisioned(ctx)
	if err != nil {
		slog.Error("the organizations with no server could not be listed", "error", err)
		return
	}
	for _, slug := range slugs {
		if _, held := c.stateOf(slug); !held {
			go c.Provision(ctx, slug)
		}
	}
}

// reprovision starts again a server the controller keeps, and does nothing for one it was told to forget. The supervisor
// decides what to restart before it takes each organization's lock, and Remove may come in between: what it removed stays
// removed.
func (c *Controller) reprovision(ctx context.Context, slug string) error {
	held := c.lock(slug)
	held.Lock()
	defer held.Unlock()
	if _, kept := c.stateOf(slug); !kept {
		return nil
	}
	return c.provision(ctx, slug)
}

// restart starts a server that has stopped or failed, and counts it under the organization's lock, so a provision that is
// under way does not write over the count. One that fails again is tried after twice as long each time.
func (c *Controller) restart(ctx context.Context, slug string) {
	held := c.lock(slug)
	held.Lock()
	defer held.Unlock()
	if c.update(slug, func(s *orgState) { s.runtime.Restarts++ }) {
		c.provision(ctx, slug)
	}
}

// alive is whether an organization's server is running on its node, and a node that cannot be
// reached is one whose servers are not known to be.
func (c *Controller) alive(slug string, runtime Runtime) bool {
	node, err := c.nodeNamed(runtime.Node)
	return err == nil && node.Alive(slug)
}
