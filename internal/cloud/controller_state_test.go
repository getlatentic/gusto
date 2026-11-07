package cloud

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// spyNode is a local node that says what the controller held when it was asked whether a server is running.
type spyNode struct {
	*LocalNode
	mu    sync.Mutex
	asked func()
	// onPorts, when set, picks the server's port.
	onPorts func(server int) int
	// started counts the servers it was asked to start.
	started atomic.Int32
}

func (n *spyNode) Alive(slug string) bool {
	n.mu.Lock()
	asked := n.asked
	n.mu.Unlock()
	if asked != nil {
		asked()
	}
	return n.LocalNode.Alive(slug)
}

func (n *spyNode) Start(spec ServerSpec) error {
	n.started.Add(1)
	return n.LocalNode.Start(spec)
}

func (n *spyNode) Ports(from int, taken map[int]bool) (int, int, error) {
	server, proxy, err := n.LocalNode.Ports(from, taken)
	if n.onPorts != nil {
		server = n.onPorts(server)
	}
	return server, proxy, err
}

func (n *spyNode) whenAsked(asked func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.asked = asked
}

func spied(c *Controller) *spyNode {
	node := &spyNode{LocalNode: c.Node.(*LocalNode)}
	c.Node = node
	return node
}

// A write for an organization that is gone, as an ended demo is, is not a failure to log and not held: the controller stops
// writing it. getlatentic/planning#744.
func TestAnEndedOrganizationsStateIsNotWrittenAndIsNotAnError(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s, _ := clocked(t, demoSet)
		c := controlled(t, s)
		_, org, err := s.StartDemo(t.Context(), "Ada", "ada@acme.com", "Acme")
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		runtime, _ := s.Runtime(t.Context(), org.Slug)
		c.Remove(t.Context(), org.Slug)
		if err := s.EndDemo(t.Context(), org.Slug, "test"); err != nil {
			t.Fatal(err)
		}
		c.Ended(org.Slug)

		if err := c.set(t.Context(), org.Slug, runtime, Starting, ""); !errors.Is(err, ErrRemoved) {
			t.Errorf("a write for an ended organization is refused as removed: %v", err)
		}
		if _, held := c.stateOf(org.Slug); held {
			t.Error("the controller holds the state of an ended organization")
		}
		if err := s.SetRuntime(t.Context(), org.Slug, runtime); !errors.Is(err, errNoOrg) {
			t.Errorf("the store refuses it by name and not by a constraint: %v", err)
		}
		if err := c.fail(t.Context(), org.Slug, runtime, errors.New("late")); !errors.Is(err, ErrRemoved) {
			t.Errorf("a late failure of an ended organization is not one: %v", err)
		}
	})
}

// The supervisor asks nodes whether servers run without holding the controller's lock, so counting and routing go on while a
// node is slow. getlatentic/planning#775.
func TestTheSupervisorDoesNotHoldTheLockWhileItAsksANode(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		node := spied(c)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		asking, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		node.whenAsked(func() {
			once.Do(func() { close(asking) })
			<-release
		})
		done := make(chan struct{})
		go func() { c.supervise(t.Context()); close(done) }()
		<-asking
		counted := make(chan int, 1)
		go func() { counted <- c.Busy() }()
		select {
		case <-counted:
		case <-time.After(5 * time.Second):
			t.Error("counting waited for a node to answer")
		}
		close(release)
		<-done
	})
}

// An organization whose state cannot be read is failed and tried again, and not left out of what the controller holds, where it
// shows as being set up until gusto cloud restarts. getlatentic/planning#775.
func TestAnOrganizationWhoseStateCannotBeReadIsFailedAndTriedAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		unreadable, cancel := context.WithCancel(t.Context())
		cancel()
		if err := c.Provision(unreadable, org.Slug); err == nil {
			t.Fatal("a state that cannot be read is an error")
		}
		if _, state := c.Route(t.Context(), org.Slug); state != Failed {
			t.Fatalf("the organization is %q, and not failed", state)
		}
		c.update(org.Slug, func(held *orgState) { held.retryAt = time.Now().Add(-time.Second) })
		c.supervise(t.Context())
		waitFor(t, func() bool { _, state := c.Route(t.Context(), org.Slug); return state == Ready })
		if held, _ := c.stateOf(org.Slug); held.runtime.Restarts != 1 {
			t.Errorf("the try was counted: %+v", held.runtime)
		}
	})
}

// What could not be written is written again by the supervisor, so the database does not stay behind what routing follows.
// getlatentic/planning#775.
func TestAStateThatCouldNotBeWrittenIsWrittenAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		runtime, _ := s.Runtime(t.Context(), org.Slug)
		refused, cancel := context.WithCancel(t.Context())
		cancel()
		if err := c.set(refused, org.Slug, runtime, Failed, "held"); err != nil {
			t.Fatal(err)
		}
		if stored, _ := s.Runtime(t.Context(), org.Slug); stored.State != Ready {
			t.Fatalf("the write was meant to fail: %+v", stored)
		}
		c.supervise(t.Context())
		if stored, _ := s.Runtime(t.Context(), org.Slug); stored.State != Failed || stored.Error != "held" {
			t.Errorf("the database follows what is held: %+v", stored)
		}
		if held, _ := c.stateOf(org.Slug); held.unsaved {
			t.Error("what was written is still to be written")
		}
	})
}

// A server that stays up has its restarts forgotten, so the backoff of a later failure starts from a second again. And one that
// has not stayed up keeps them. getlatentic/planning#775.
func TestARunningServersRestartsAreForgottenOnceItHasStayedUp(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		c.Stable = 50 * time.Millisecond
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		c.update(org.Slug, func(held *orgState) { held.runtime.Restarts = 3 })
		c.supervise(t.Context())
		if held, _ := c.stateOf(org.Slug); held.runtime.Restarts != 3 {
			t.Fatalf("a server that has just started keeps its restarts: %+v", held.runtime)
		}
		time.Sleep(c.Stable + 20*time.Millisecond)
		c.supervise(t.Context())
		held, _ := c.stateOf(org.Slug)
		stored, _ := s.Runtime(t.Context(), org.Slug)
		if held.runtime.Restarts != 0 || stored.Restarts != 0 {
			t.Errorf("held %+v, stored %+v", held.runtime, stored)
		}
	})
}

// A restart is counted under the organization's lock, so a provision under way does not write over it.
// getlatentic/planning#775.
func TestARestartCountedWhileAProvisionIsUnderWayIsKept(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s, "FAKE_DELAY=600ms")
		org := made(t, s, "ada@acme.com", "Acme")
		provisioned := make(chan error, 1)
		go func() { provisioned <- c.Provision(t.Context(), org.Slug) }()
		waitFor(t, func() bool { return c.Busy() == 1 })
		c.restart(t.Context(), org.Slug)
		if err := <-provisioned; err != nil {
			t.Fatal(err)
		}
		if held, _ := c.stateOf(org.Slug); held.runtime.Restarts != 1 || held.runtime.State != Ready {
			t.Errorf("the restart was lost: %+v", held.runtime)
		}
		c.supervise(t.Context())
		if stored, _ := s.Runtime(t.Context(), org.Slug); stored.Restarts != 1 {
			t.Errorf("the restart is written down: %+v", stored)
		}
	})
}

// Asking to provision an organization whose server runs does not turn its visitors away, and does nothing to the server.
// getlatentic/planning#775.
func TestProvisioningARunningServerAgainLeavesItReady(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		node := spied(c)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		pid := whoIs(t, mustRoute(t, c, org.Slug))["pid"]
		starts := node.started.Load()
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		if node.started.Load() != starts {
			t.Error("the organization was set up again, and was not ready meanwhile")
		}
		if _, state := c.Route(t.Context(), org.Slug); state != Ready {
			t.Errorf("%q", state)
		}
		if again := whoIs(t, mustRoute(t, c, org.Slug))["pid"]; again != pid {
			t.Error("the server was started again")
		}
	})
}

// A gusto cloud server that starts does not write down a server that runs on a host apart as starting: the other gusto cloud servers
// go on routing to it, and provisioning finds it running. getlatentic/planning#775.
func TestARunningServerIsNotWrittenAsStartingWhenGustoCloudStarts(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		first := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := first.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		second := controlled(t, s)
		second.Node = first.Node
		orgs, _ := s.Orgs(t.Context())
		second.resume(t.Context(), orgs)
		if stored, _ := s.Runtime(t.Context(), org.Slug); stored.State != Ready {
			t.Errorf("a running server was written down as %q", stored.State)
		}
		if _, state := second.Route(t.Context(), org.Slug); state != Starting {
			t.Errorf("until it is found running, it is %q", state)
		}
		if err := second.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		if _, state := second.Route(t.Context(), org.Slug); state != Ready {
			t.Errorf("found running, it is %q", state)
		}
	})
}

func mustRoute(t *testing.T, c *Controller, slug string) string {
	t.Helper()
	address, state := c.Route(t.Context(), slug)
	if state != Ready {
		t.Fatalf("%s is %q", slug, state)
	}
	return address
}
