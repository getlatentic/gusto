package cloud

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"
)

// orgState is what the controller holds of one organization it serves: its runtime, when it is next tried if it failed or
// waits, since when it has been ready, and whether its runtime is yet to be written to the database. put is the one place it
// changes, and what is derived from the runtime is derived there.
type orgState struct {
	runtime Runtime
	retryAt time.Time
	readyAt time.Time
	unsaved bool
}

// stateOf is what the controller holds of an organization, and whether it holds it.
func (c *Controller) stateOf(slug string) (orgState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	held, found := c.orgs[slug]
	return held, found
}

// forget drops what the controller holds of an organization.
func (c *Controller) forget(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.orgs, slug)
}

// put holds a runtime for an organization. saved says the database has it. A failed organization is tried again after a
// backoff that grows with its restarts, and a waiting one after WaitEvery.
func (c *Controller) put(slug string, runtime Runtime, saved bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.orgs == nil {
		c.orgs = map[string]orgState{}
	}
	before := c.orgs[slug]
	next := orgState{runtime: runtime, unsaved: !saved}
	now := time.Now()
	switch runtime.State {
	case Ready:
		next.readyAt = now
		if before.runtime.State == Ready {
			next.readyAt = before.readyAt
		}
	case Failed:
		next.retryAt = now.Add(backoff(runtime.Restarts))
	case Waiting:
		next.retryAt = now.Add(c.waitEvery())
	}
	c.orgs[slug] = next
}

// update changes what is held of an organization the controller holds, and says whether it held it.
func (c *Controller) update(slug string, change func(*orgState)) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	held, found := c.orgs[slug]
	if found {
		change(&held)
		c.orgs[slug] = held
	}
	return found
}

// backoff is how long a server that failed waits before it is started again: a second for the first failure, twice as long
// for each restart since, up to five minutes.
func backoff(restarts int) time.Duration {
	return time.Duration(min(1<<min(restarts, 8), 300)) * time.Second
}

// set writes down the state of a server and holds it for routing. A write refused because another gusto cloud server leads now
// is said, and nothing is held: the provision that made it stops. So is a write for an organization that is gone, which is
// forgotten and is ErrRemoved. Any other failure to write is logged, and routing follows what is held, which is written again by
// the supervisor.
func (c *Controller) set(ctx context.Context, slug string, runtime Runtime, state State, why string) error {
	runtime.State, runtime.Error = state, why
	err := c.Store.SetRuntimeAt(ctx, c.epoch.Load(), slug, runtime)
	switch {
	case errors.Is(err, ErrNotLeading):
		return err
	case errors.Is(err, errNoOrg):
		c.forget(slug)
		return ErrRemoved
	case err != nil:
		slog.Error("an organization's state was not written, and is written again", "org", slug, "error", err)
	}
	c.put(slug, runtime, err == nil)
	return nil
}

// wait leaves an organization waiting for a node with room, which it is tried for again shortly. It is not a failure
// and is not counted as one, since what it waits for is being made.
func (c *Controller) wait(ctx context.Context, slug string, runtime Runtime, err error) error {
	c.set(ctx, slug, runtime, Waiting, err.Error())
	if c.OnWaiting != nil {
		c.OnWaiting()
	}
	return err
}

func (c *Controller) waitEvery() time.Duration {
	if c.WaitEvery > 0 {
		return c.WaitEvery
	}
	return 500 * time.Millisecond
}

// fail leaves an organization failed, with the reason, for the supervisor to try again. One that is gone is not failed: it is
// ErrRemoved.
func (c *Controller) fail(ctx context.Context, slug string, runtime Runtime, err error) error {
	if errors.Is(err, errNoOrg) {
		c.forget(slug)
		return ErrRemoved
	}
	slog.Error("an organization's server could not be made ready", "org", slug, "error", err)
	if set := c.set(ctx, slug, runtime, Failed, err.Error()); errors.Is(set, ErrRemoved) {
		return set
	}
	return err
}

// unreadable fails an organization whose state could not be read, in memory only: the database still holds what it did, and
// writing a runtime that was never read could wipe its ports. Held as failed it is tried again, where it would otherwise
// show as being set up until gusto cloud restarts.
func (c *Controller) unreadable(slug string, err error) {
	slog.Error("an organization's state could not be read, so it is tried again", "org", slug, "error", err)
	held, _ := c.stateOf(slug)
	runtime := held.runtime
	runtime.State, runtime.Error = Failed, err.Error()
	c.put(slug, runtime, true)
}

// Waiting is how many organizations are waiting for a node with room, which is what makes another start.
func (c *Controller) Waiting() int { return c.count(Waiting) }

// Busy is how many organizations wait for a node with room or are being placed and started, while which no node is idle:
// the one an organization is being placed on may be the one with nothing on it yet.
func (c *Controller) Busy() int { return c.count(Waiting, Provisioning, Starting) }

func (c *Controller) count(states ...State) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	counted := 0
	for _, held := range c.orgs {
		if slices.Contains(states, held.runtime.State) {
			counted++
		}
	}
	return counted
}
