package cloud

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A server that answers and then dies is restarted at once the first time, and then no sooner than the backoff after it was last
// started, where it was restarted at every sweep. getlatentic/planning#832.
func TestAServerThatAnswersAndDiesAgainIsRestartedByTheBackoff(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		node := spied(c)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		dies := func() {
			t.Helper()
			node.Stop(org.Slug, time.Second)
			c.supervise(t.Context())
		}

		dies()
		if got := node.started.Load(); got != 2 {
			t.Fatalf("a server that had stayed up was not restarted at once: %d starts", got)
		}
		waitFor(t, func() bool { _, state := c.Route(t.Context(), org.Slug); return state == Ready })

		dies()
		if got := node.started.Load(); got != 2 {
			t.Fatalf("a server that died again at once was restarted at the next sweep: %d starts", got)
		}
		for range 3 {
			c.supervise(t.Context())
		}
		if got := node.started.Load(); got != 2 {
			t.Fatalf("sweeps inside the backoff restarted it: %d starts", got)
		}
		c.update(org.Slug, func(held *orgState) { held.readyAt = held.readyAt.Add(-backoff(held.runtime.Restarts)) })
		c.supervise(t.Context())
		if got := node.started.Load(); got != 3 {
			t.Fatalf("it was not restarted once the backoff had passed: %d starts", got)
		}
	})
}

func TestTheDelayBeforeARestartGrowsWithTheRestartsAndTheFirstHasNone(t *testing.T) {
	if restartDelay(0) != 0 {
		t.Errorf("the first restart waits %s", restartDelay(0))
	}
	for restarts := 1; restarts < 12; restarts++ {
		if restartDelay(restarts) != backoff(restarts) || restartDelay(restarts) < restartDelay(restarts-1) {
			t.Errorf("restart %d waits %s after %s", restarts, restartDelay(restarts), restartDelay(restarts-1))
		}
	}
}

// aHolder listens on a port of the loopback until the test ends, as another process does.
func aHolder(t *testing.T, port int) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

// provisionedAndStopped is an organization with a running server that is then stopped, so its stored ports are free to be taken.
func provisionedAndStopped(t *testing.T, s *Store, c *Controller) (Org, Runtime) {
	t.Helper()
	org := made(t, s, "ada@acme.com", "Acme")
	if err := c.Provision(t.Context(), org.Slug); err != nil {
		t.Fatal(err)
	}
	c.Node.Stop(org.Slug, time.Second)
	runtime, err := s.Runtime(t.Context(), org.Slug)
	if err != nil {
		t.Fatal(err)
	}
	return org, runtime
}

// A server whose stored port another process holds is given another, and starts: it does not fail on that port until the other
// process lets go. Its proxy port, which no computer was told, stays. getlatentic/planning#833.
func TestAServerWhoseServerPortAnotherProcessHoldsIsGivenAnotherAndKeepsItsProxyPort(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		c.PortGrace = 50 * time.Millisecond
		org, was := provisionedAndStopped(t, s, c)
		registry, _ := s.Computers(org.Slug)
		told, _, _ := registry.Ensure("ada@acme.com", "", "local")
		aHolder(t, was.Port)

		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatalf("a server whose port is held did not start: %v", err)
		}
		now, _ := s.Runtime(t.Context(), org.Slug)
		if now.State != Ready || now.Port == was.Port || now.ProxyPort != was.ProxyPort {
			t.Errorf("was %d and %d, now %+v", was.Port, was.ProxyPort, now)
		}
		if whoIs(t, mustRoute(t, c, org.Slug))["org"] != org.Slug {
			t.Error("the server does not answer on its new port")
		}
		if retiring, _ := registry.Retiring(); len(retiring) != 0 {
			t.Errorf("a computer was removed for a port it was not told: %+v, %s", retiring, told.ID)
		}
	})
}

// A held proxy port is the one a computer was told, so the computers of the organization are removed to be made again with the new.
func TestAServerWhoseProxyPortAnotherProcessHoldsIsGivenAnotherAndItsComputersAreRemoved(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		c.PortGrace = 50 * time.Millisecond
		org, was := provisionedAndStopped(t, s, c)
		registry, _ := s.Computers(org.Slug)
		told, _, _ := registry.Ensure("ada@acme.com", "", "local")
		aHolder(t, was.ProxyPort)

		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatalf("a server whose proxy port is held did not start: %v", err)
		}
		now, _ := s.Runtime(t.Context(), org.Slug)
		if now.State != Ready || now.ProxyPort == was.ProxyPort || now.Port != was.Port {
			t.Errorf("was %d and %d, now %+v", was.Port, was.ProxyPort, now)
		}
		retiring, _ := registry.Retiring()
		if len(retiring) != 1 || retiring[0].ID != told.ID {
			t.Errorf("the computer that was told the old proxy port is asked to be removed: %+v", retiring)
		}
	})
}

// A port held for a moment, by the server that was there before, is waited for and not moved.
func TestAPortHeldForAMomentIsWaitedForAndNotMoved(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		c.PortGrace = 5 * time.Second
		org, was := provisionedAndStopped(t, s, c)
		registry, _ := s.Computers(org.Slug)
		registry.Ensure("ada@acme.com", "", "local")
		holder := aHolder(t, was.ProxyPort)
		time.AfterFunc(300*time.Millisecond, func() { holder.Close() })

		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		now, _ := s.Runtime(t.Context(), org.Slug)
		if now.Port != was.Port || now.ProxyPort != was.ProxyPort {
			t.Errorf("ports moved for a port that was let go: was %d and %d, now %d and %d", was.Port, was.ProxyPort, now.Port, now.ProxyPort)
		}
		if retiring, _ := registry.Retiring(); len(retiring) != 0 {
			t.Errorf("a computer was removed: %+v", retiring)
		}
	})
}

// A server that is still running holds its own ports, which are not another process's.
func TestARunningServersOwnPortsAreNotHeldByAnotherProcess(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		c.PortGrace = 50 * time.Millisecond
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		was, _ := s.Runtime(t.Context(), org.Slug)
		moved, err := c.moveHeldPorts(t.Context(), org.Slug, was, c.Node)
		if err != nil || moved.Port != was.Port || moved.ProxyPort != was.ProxyPort {
			t.Errorf("a running server's ports moved: %+v %v", moved, err)
		}
	})
}

// StopAll waits for a server that is being started, and stops it, where it was missed and then started after.
// getlatentic/planning#840.
func TestStopAllWaitsForAServerBeingStartedAndStopsIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		node := spied(c)
		entered, release := make(chan struct{}), make(chan struct{})
		node.beforeStart = func() { close(entered); <-release }
		org := made(t, s, "ada@acme.com", "Acme")
		provisioned := make(chan error, 1)
		go func() { provisioned <- c.Provision(t.Context(), org.Slug) }()
		<-entered

		stopped := make(chan struct{})
		go func() { c.StopAll(); close(stopped) }()
		select {
		case <-stopped:
			t.Fatal("StopAll did not wait for the server being started, which then starts after it")
		case <-time.After(200 * time.Millisecond):
		}
		close(release)
		<-stopped
		<-provisioned
		if node.Alive(org.Slug) {
			t.Error("a server that was being started outlived StopAll")
		}
	})
}

// Nothing is started once StopAll has run, and what was refused is not written as a failure.
func TestNoServerIsStartedAfterStopAll(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		node := spied(c)
		org := made(t, s, "ada@acme.com", "Acme")
		c.StopAll()
		if err := c.Provision(t.Context(), org.Slug); !errors.Is(err, errStopped) {
			t.Fatalf("provisioning after StopAll said %v", err)
		}
		if node.started.Load() != 0 || node.Alive(org.Slug) {
			t.Error("a server was started after StopAll")
		}
		if stored, _ := s.Runtime(t.Context(), org.Slug); stored.State == Failed {
			t.Errorf("a start refused for stopping was written as a failure: %+v", stored)
		}
	})
}

// deadNode runs nothing, as a start that lost its port to another server of the same organization has nothing running.
type deadNode struct {
	*LocalNode
	port int
}

func (n deadNode) Ports(int, map[int]bool) (int, int, error) { return n.port, n.port + 1, nil }
func (deadNode) Start(ServerSpec) error                      { return nil }
func (deadNode) Alive(string) bool                           { return false }

// The organization's own server answers on the port, but not the one this node started: the start did not get the port, and
// is not taken as having it. getlatentic/planning#840.
func TestAnAnswerFromAServerThisNodeDidNotStartIsNotAReadyServer(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		c.Wait = 5 * time.Second
		org := made(t, s, "ada@acme.com", "Acme")
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/keys/") {
				key, err := os.ReadFile(filepath.Join(s.DataDir(org.Slug), "keys", org.Slug+".pub"))
				if err != nil {
					http.NotFound(w, r)
					return
				}
				w.Write(key)
				return
			}
			w.Write([]byte("ok\n"))
		}))
		defer other.Close()
		port, _ := strconv.Atoi(other.URL[strings.LastIndex(other.URL, ":")+1:])
		c.Node = deadNode{LocalNode: &LocalNode{}, port: port}
		err := c.Provision(t.Context(), org.Slug)
		if err == nil || !strings.Contains(err.Error(), "ended as it started") {
			t.Fatalf("another server's answer made the start ready: %v", err)
		}
		if _, state := c.Route(t.Context(), org.Slug); state != Failed {
			t.Errorf("%q", state)
		}
	})
}

// resume does not put an organization the controller holds back to starting over a newer state, nor one being provisioned.
// getlatentic/planning#840.
func TestResumeLeavesAnOrganizationTheControllerHoldsOrIsProvisioning(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{PerDomain: 100})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		orgs, _ := s.Orgs(t.Context())
		c.resume(t.Context(), orgs)
		if _, state := c.Route(t.Context(), org.Slug); state != Ready {
			t.Errorf("an organization that was ready and held is %q after resume", state)
		}

		busy := made(t, s, "bob@globex.com", "Globex")
		held := c.lock(busy.Slug)
		held.Lock()
		orgs, _ = s.Orgs(t.Context())
		resumed := make(chan struct{})
		go func() { c.resume(context.Background(), orgs); close(resumed) }()
		select {
		case <-resumed:
		case <-time.After(5 * time.Second):
			t.Fatal("resume waited for an organization that is being provisioned")
		}
		if _, known := c.stateOf(busy.Slug); known {
			t.Error("resume held a state for an organization that is being provisioned")
		}
		held.Unlock()
	})
}
