package cloud

import (
	"context"
	"log/slog"
	"time"
)

// givePorts gives an organization two ports on its node and writes them down before anyone
// else is given any, so two organizations provisioned together do not share one.
func (c *Controller) givePorts(ctx context.Context, slug string, runtime Runtime, node Node) (Runtime, error) {
	return c.choosePorts(ctx, slug, runtime, node, true, true)
}

// choosePorts replaces the organization's server port, its proxy port, or both, with ports on its node that no organization has,
// itself included, and writes them down. The ports it had are given to nobody else, so the new ones are not the old.
func (c *Controller) choosePorts(ctx context.Context, slug string, runtime Runtime, node Node, server, proxy bool) (Runtime, error) {
	c.portMu.Lock()
	defer c.portMu.Unlock()
	orgs, err := c.Store.Orgs(ctx)
	if err != nil {
		return runtime, err
	}
	given := map[int]bool{}
	for _, org := range orgs {
		if other, err := c.Store.Runtime(ctx, org.Slug); err == nil && other.Node == runtime.Node {
			given[other.Port], given[other.ProxyPort] = true, true
		}
	}
	first, second, err := node.Ports(c.PortFrom, given)
	if err != nil {
		return runtime, err
	}
	if server {
		runtime.Port, first = first, second
	}
	if proxy {
		runtime.ProxyPort = first
	}
	return runtime, c.Store.SetRuntimeAt(ctx, c.epoch.Load(), slug, runtime)
}

// moveHeldPorts gives a server that is not running other ports in place of those another process holds, which it would end
// on, as it starts, for as long as that process lives. Only a port that is held is replaced. The proxy port is the one a
// computer is told when it is made, so a computer of the organization is removed, and made again with the new one, when it
// moves; the server port is told to none.
func (c *Controller) moveHeldPorts(ctx context.Context, slug string, runtime Runtime, node Node) (Runtime, error) {
	if node.Alive(slug) {
		return runtime, nil
	}
	server, proxy := c.heldAfterGrace(ctx, node, runtime)
	if !server && !proxy {
		return runtime, nil
	}
	slog.Warn("another process holds a port of an organization's server, so it is given another",
		"org", slug, "port", runtime.Port, "held", server, "proxyPort", runtime.ProxyPort, "proxyHeld", proxy)
	if proxy {
		if err := c.retireComputers(slug); err != nil {
			return runtime, err
		}
	}
	return c.choosePorts(ctx, slug, runtime, node, server, proxy)
}

// heldAfterGrace says which of an organization's ports another process still holds once PortGrace has passed: a port held for
// a moment is the end of the server that was there before, which is not a reason to move.
func (c *Controller) heldAfterGrace(ctx context.Context, node Node, runtime Runtime) (server, proxy bool) {
	deadline := time.Now().Add(c.portGrace())
	for {
		server, proxy = node.Held(runtime.Port), node.Held(runtime.ProxyPort)
		if (!server && !proxy) || !time.Now().Before(deadline) {
			return server, proxy
		}
		select {
		case <-ctx.Done():
			return false, false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (c *Controller) portGrace() time.Duration {
	if c.PortGrace > 0 {
		return c.PortGrace
	}
	return 10 * time.Second
}

// retireComputers asks for every computer of the organization to be removed, which its server does once it runs.
func (c *Controller) retireComputers(slug string) error {
	registry, err := c.Store.Computers(slug)
	if err != nil {
		return err
	}
	return registry.RetireAll()
}
