package cloud

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Controller makes each organization's server exist and keeps it running. An
// organization is provisioned when it is made and again when gusto cloud starts,
// by steps that are each safe to run again, and the state of each is written down as
// it goes, so a crash between two steps leaves something the next pass finishes.
type Controller struct {
	Store *Store
	// Node is the one place servers run, when Nodes is not set.
	Node Node
	// Nodes is where servers may run when there is more than one. An organization is placed on
	// one when it is made and stays there, because its computers are on that node's gateway.
	Nodes NodeSet
	// Command is the program that is an organization's server, and its arguments.
	Command []string
	// Env is the settings every organization's server has, and only those and
	// its own: the operator's credentials directory, models and sandbox.
	Env []string
	// DemoEnv is what a demo organization's server has on top of Env, and wins over it: a demo's agents can run in a sandbox
	// of another kind than a sign-up's, such as a microVM for each person where a sign-up has a container.
	DemoEnv []string
	// CloudURL is gusto cloud's public address, which a server sends people to.
	CloudURL string
	// OrgURL is the address each organization's server is reached at, which an app a person signs in to sends them back to. With
	// none, people sign in to no apps.
	OrgURL func(slug string) string
	// PortFrom is the first port organizations' servers are given, two each.
	PortFrom int
	// Wait is how long a server has to answer once it is started.
	Wait time.Duration
	// Concurrent is how many tasks a server's queue runs at once.
	Concurrent int
	// StopGrace is how long a server is given to end its tasks' records when it is
	// asked to stop, before it is made to.
	StopGrace time.Duration
	// IndexDir is where organizations' indexes are kept, in a directory for each, on the local disk of the host a server
	// runs on. It is the index directory under the store's root when not set.
	IndexDir string
	// WaitEvery is how often an organization waiting for a node is tried again. It is half a second when not set: a node
	// says it is ready by writing to the shared file system, which cannot tell a reader it changed, and a visitor waits
	// on the organization.
	WaitEvery time.Duration
	// OnWaiting is told when an organization starts waiting for a node with room, so a node can be started for it at
	// once. It is called outside the controller's lock and must not block.
	OnWaiting func()
	// Metrics counts provisions, and reports the organizations held by state. Nothing is counted when it is nil.
	Metrics *Metrics
	// Stable is how long a server has to stay ready before the restarts it needed are forgotten, two minutes when not set. A
	// server that stops again sooner is restarted less and less often.
	Stable time.Duration
	// PortGrace is how long a port of an organization's server that another process holds is waited for before it is given
	// another, ten seconds when not set: the process may be the previous gusto cloud server's server for it, which is ending.
	PortGrace time.Duration

	// mu guards the maps below and is held only to read or change them, never across a call to a node or the database, so
	// routing never waits on a server that is starting. Each organization is provisioned by one caller at a time, under its
	// own lock, and ports are given out under portMu, so two do not choose the same. Only the leader provisions, and the
	// lease's epoch fences what it writes, so two gusto cloud servers do not give two organizations one port.
	mu   sync.Mutex
	orgs map[string]orgState
	// removed is when each organization Remove forgot was made, until Ended: a slug may be given again, and a new
	// organization that holds it is told apart from the removed one by when it was made.
	removed map[string]time.Time
	locks   sync.Map
	portMu  sync.Mutex
	// stopped is set by StopAll, under mu, and starts counts the servers being started. A server is started only by one that
	// counted itself in while stopped was false, so StopAll, which waits for the count, finds every server it must stop.
	stopped bool
	starts  sync.WaitGroup

	// epoch is the lease this controller leads with, which fences what it writes: 0 when no lease governs it, and -1
	// while another gusto cloud server leads, when this one routes from the database and starts nothing.
	epoch atomic.Int64
	// followed is what routing read from the database while another gusto cloud server leads.
	followed routeCache
}

// Lead makes this controller the one that provisions and supervises, at the lease's epoch.
func (c *Controller) Lead(epoch int64) { c.epoch.Store(epoch) }

// Follow makes this controller route from the database and start nothing, while another gusto cloud server leads. What it kept
// while it led is forgotten, since the leader changes it now.
func (c *Controller) Follow() {
	c.epoch.Store(-1)
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.orgs)
}

func (c *Controller) following() bool { return c.epoch.Load() < 0 }

func (c *Controller) lock(slug string) *sync.Mutex {
	held, _ := c.locks.LoadOrStore(slug, &sync.Mutex{})
	return held.(*sync.Mutex)
}

func (c *Controller) indexDir() string {
	if c.IndexDir != "" {
		return c.IndexDir
	}
	return filepath.Join(c.Store.Root(), "index")
}

func (c *Controller) orgDir(slug string) string { return c.Store.OrgDir(slug) }

// Provision makes an organization's server exist and start, and says when it answers. A gusto cloud server that does not lead
// starts nothing: the leader finds the organization and provisions it.
func (c *Controller) Provision(ctx context.Context, slug string) error {
	if c.following() {
		return nil
	}
	held := c.lock(slug)
	held.Lock()
	defer held.Unlock()
	return c.provision(ctx, slug)
}

// ErrRemoved is the answer to provisioning an organization that is gone, or that Remove forgot.
var ErrRemoved = errors.New("that organization has been removed")

func (c *Controller) provision(ctx context.Context, slug string) error {
	runtime, running, err := c.begin(ctx, slug)
	if err != nil || running {
		return err
	}
	began := time.Now()
	err = c.provisionSteps(ctx, slug, runtime)
	c.Metrics.provisioned(time.Since(began), err)
	return err
}

func (c *Controller) provisionSteps(ctx context.Context, slug string, runtime Runtime) error {
	if err := c.set(ctx, slug, runtime, Provisioning, ""); err != nil {
		return err
	}
	if err := c.prepare(ctx, slug); err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	node, err := c.place(ctx, slug, &runtime)
	if errors.Is(err, ErrNoCapacity) {
		return c.wait(ctx, slug, runtime, err)
	}
	if err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	if err := c.set(ctx, slug, runtime, Starting, ""); err != nil {
		return err
	}
	if err := c.start(ctx, slug, runtime, node); err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	return c.becomeReady(ctx, slug, runtime)
}

// becomeReady records the server as ready, once it answers, with the command it was started with. A demo's time starts when its
// server first answers, whoever opens it first: its visitor's waiting page, or a colleague who came by the organization's own
// address. The server is not ready until the time is written, and is tried again if it cannot be.
func (c *Controller) becomeReady(ctx context.Context, slug string, runtime Runtime) error {
	runtime.Command = strings.Join(c.Command, " ")
	if _, err := c.Store.DemoReady(ctx, slug); err != nil && !errors.Is(err, ErrNotDemo) {
		return c.fail(ctx, slug, runtime, fmt.Errorf("a demo's time could not be started: %w", err))
	}
	return c.set(ctx, slug, runtime, Ready, "")
}

// begin reads what provisioning goes on from: the runtime in the database, with the restarts this controller has counted,
// which are ahead of the database's. running says the server is running as this release already, and that is all there is
// to do. An organization whose state cannot be read is failed in memory, to be tried again.
func (c *Controller) begin(ctx context.Context, slug string) (runtime Runtime, running bool, err error) {
	if err = c.stillServed(ctx, slug); err == nil {
		runtime, err = c.Store.Runtime(ctx, slug)
	}
	if err != nil {
		if !errors.Is(err, ErrRemoved) {
			c.unreadable(slug, err)
		}
		return Runtime{}, false, err
	}
	stored := runtime.Restarts
	if held, known := c.stateOf(slug); known {
		runtime.Restarts = held.runtime.Restarts
	}
	if running = c.runsThisRelease(slug, runtime); running {
		c.put(slug, runtime, runtime.Restarts == stored)
	}
	return runtime, running, nil
}

// runsThisRelease says an organization's server is ready and running, started with this gusto cloud server's command. Asking
// it to provision again leaves it as it is: marking it as being set up would turn its visitors away for nothing.
func (c *Controller) runsThisRelease(slug string, runtime Runtime) bool {
	return runtime.State == Ready && runtime.Command == strings.Join(c.Command, " ") && c.alive(slug, runtime)
}

// place puts an organization on a node, and gives it ports there, writing each down.
func (c *Controller) place(ctx context.Context, slug string, runtime *Runtime) (Node, error) {
	node, err := c.placement(ctx, slug, runtime)
	if err != nil {
		return nil, err
	}
	if runtime.Port == 0 {
		*runtime, err = c.givePorts(ctx, slug, *runtime, node)
	} else {
		*runtime, err = c.moveHeldPorts(ctx, slug, *runtime, node)
	}
	return node, err
}

// start runs an organization's server, and returns when it answers as the organization's.
func (c *Controller) start(ctx context.Context, slug string, runtime Runtime, node Node) error {
	env, err := c.environment(ctx, slug, runtime, node)
	if err != nil {
		return err
	}
	spec := ServerSpec{Slug: slug, Command: c.Command, Env: env, Log: filepath.Join(c.orgDir(slug), "server.log")}
	if err := c.launch(node, spec, runtime); err != nil {
		return err
	}
	return c.answers(ctx, node, spec, runtime.Port)
}

// errStopped is what starting a server says once StopAll has run: gusto cloud is ending, and a server started now would outlive it.
var errStopped = errors.New("gusto cloud is stopping, so no server is started")

// launch runs the server unless gusto cloud is stopping. StopAll waits for the launches that began before it, so none starts a
// server after it has looked for the ones to stop.
func (c *Controller) launch(node Node, spec ServerSpec, runtime Runtime) error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return errStopped
	}
	c.starts.Add(1)
	c.mu.Unlock()
	defer c.starts.Done()
	if err := c.endOtherRelease(node, spec.Slug, runtime); err != nil {
		return err
	}
	return node.Start(spec)
}

// stillServed says whether an organization is one to serve: it is in the store, and it is not the one Remove forgot. A call
// to provision can be decided on before Remove and run after it, as the signup's and Run's are; this is where it is refused.
func (c *Controller) stillServed(ctx context.Context, slug string) error {
	org, found, err := c.Store.Org(ctx, slug)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	made, removed := c.removed[slug]
	ending := found && org.Demo != nil && org.Demo.Ending
	if found && !ending && !(removed && made.Equal(org.Created)) {
		return nil
	}
	delete(c.orgs, slug)
	return ErrRemoved
}

// endOtherRelease stops an organization's server that was started with another command than this gusto cloud server's, so that
// it is started again as this release. A server on a host apart outlives gusto cloud that started it, and would
// otherwise run the release it was started with until its host goes.
func (c *Controller) endOtherRelease(node Node, slug string, runtime Runtime) error {
	if runtime.Command == strings.Join(c.Command, " ") || !node.Alive(slug) {
		return nil
	}
	slog.Info("an organization's server is of another release, so it is started again", "org", slug, "was", runtime.Command)
	return node.Stop(slug, c.grace())
}

// nodeNamed is the node an organization was placed on.
func (c *Controller) nodeNamed(name string) (Node, error) {
	if c.Nodes == nil {
		return c.Node, nil
	}
	return c.Nodes.Get(name)
}

// placement is the node an organization's server runs on. One that was placed stays there, since its
// computers are on that node's gateway, and one that was not is put on the node with the most room and
// has that written down before anything is started on it.
func (c *Controller) placement(ctx context.Context, slug string, runtime *Runtime) (Node, error) {
	if runtime.Node != "" {
		node, err := c.nodeNamed(runtime.Node)
		if !errors.Is(err, ErrNodeGone) {
			return node, err
		}
		slog.Warn("an organization's node is gone, so it is placed on another", "org", slug, "node", runtime.Node)
		// The ports were the gone node's, and are not this one's.
		runtime.Node, runtime.Port, runtime.ProxyPort = "", 0, 0
	}
	node := c.Node
	if c.Nodes != nil {
		var err error
		if node, err = c.Nodes.Place(); err != nil {
			return nil, err
		}
	}
	runtime.Node = node.Name()
	return node, c.Store.SetRuntimeAt(ctx, c.epoch.Load(), slug, *runtime)
}

// Route is where an organization's server is when it can be reached, and its state
// when it cannot. A server whose process has gone is not reached.
func (c *Controller) Route(ctx context.Context, slug string) (address string, state State) {
	if c.following() {
		return c.routeFollowed(ctx, slug)
	}
	held, known := c.stateOf(slug)
	if !known {
		return "", ""
	}
	runtime := held.runtime
	if runtime.State != Ready {
		return "", runtime.State
	}
	node, err := c.nodeNamed(runtime.Node)
	if err != nil || !node.Alive(slug) {
		return "", Starting
	}
	return net.JoinHostPort(node.Host(), strconv.Itoa(runtime.Port)), Ready
}

// StopAll stops every server this controller holds, when gusto cloud stops. A server on a host apart is left running:
// it does not end with gusto cloud, which is restarted at every deploy.
func (c *Controller) StopAll() {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
	c.starts.Wait()
	c.mu.Lock()
	held := make(map[string]Runtime, len(c.orgs))
	for slug, state := range c.orgs {
		held[slug] = state.runtime
	}
	c.mu.Unlock()
	var wg sync.WaitGroup
	for slug, runtime := range held {
		wg.Add(1)
		go func() {
			defer wg.Done()
			node, err := c.nodeNamed(runtime.Node)
			if err != nil {
				return
			}
			// A server on another host does not end with gusto cloud, which is restarted at every deploy, and is
			// found running by the one that starts.
			if apart, ok := node.(interface{ Detached() bool }); ok && apart.Detached() {
				return
			}
			node.Stop(slug, c.grace())
		}()
	}
	wg.Wait()
}

// Remove stops an organization's server and forgets it, so it is not started again, even by a call to provision made
// before. What the organization's directory holds is the store's to delete, after this, and Ended is called once it has.
func (c *Controller) Remove(ctx context.Context, slug string) {
	held := c.lock(slug)
	held.Lock()
	defer held.Unlock()
	org, _, err := c.Store.Org(ctx, slug)
	if err != nil {
		slog.Error("a removed organization could not be read, so a new one at its address is refused until it ends", "org", slug, "error", err)
	}
	c.mu.Lock()
	runtime := c.orgs[slug].runtime
	delete(c.orgs, slug)
	if c.removed == nil {
		c.removed = map[string]time.Time{}
	}
	c.removed[slug] = org.Created
	c.mu.Unlock()
	if runtime.Node == "" {
		if stored, err := c.Store.Runtime(ctx, slug); err == nil {
			runtime = stored
		}
	}
	if node, err := c.nodeNamed(runtime.Node); err == nil {
		node.Stop(slug, c.grace())
	}
}

// Ended says the store has deleted an organization Remove forgot. The store is then what refuses it, and its address is
// free for a new organization.
func (c *Controller) Ended(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.removed, slug)
}

func (c *Controller) grace() time.Duration {
	if c.StopGrace > 0 {
		return c.StopGrace
	}
	return 30 * time.Second
}
