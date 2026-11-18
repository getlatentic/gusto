package cloud

import (
	"context"
	"time"
)

// Changed is closed when the state of an organization this controller holds next changes. A gusto cloud server that does not
// lead makes no change and hears of none, so what it is given closes after routeFresh, when routing reads the database again.
func (c *Controller) Changed() <-chan struct{} {
	if c.following() {
		later := make(chan struct{})
		time.AfterFunc(routeFresh, func() { close(later) })
		return later
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.changed == nil {
		c.changed = make(chan struct{})
	}
	return c.changed
}

// changedNow tells everyone waiting on Changed that a state changed. c.mu is held.
func (c *Controller) changedNow() {
	if c.changed != nil {
		close(c.changed)
		c.changed = nil
	}
}

// AwaitChange is an organization's state once it is not seen, or when most has passed or ctx ends, whichever is first.
func (c *Controller) AwaitChange(ctx context.Context, slug string, seen State, most time.Duration) State {
	deadline := time.NewTimer(most)
	defer deadline.Stop()
	for {
		// Changed is asked for before the state is read, so a change between the two is not missed.
		changed := c.Changed()
		if _, state := c.Route(ctx, slug); state != seen {
			return state
		}
		select {
		case <-changed:
		case <-deadline.C:
			return seen
		case <-ctx.Done():
			return seen
		}
	}
}

// Wake tries again at once every organization that waits for a node, as when a node says it has started. It does not block.
func (c *Controller) Wake() {
	c.mu.Lock()
	for slug, held := range c.orgs {
		if held.runtime.State == Waiting {
			held.retryAt = time.Time{}
			c.orgs[slug] = held
		}
	}
	woken := c.wakeup()
	c.mu.Unlock()
	select {
	case woken <- struct{}{}:
	default:
	}
}

// wakeup is the channel Wake sends on and Run listens to. c.mu is held.
func (c *Controller) wakeup() chan struct{} {
	if c.wake == nil {
		c.wake = make(chan struct{}, 1)
	}
	return c.wake
}
