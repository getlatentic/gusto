package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlatentic/gusto/internal/signing"
)

// Controller makes each organization's server exist and keeps it running. An
// organization is provisioned when it is made and again when the front door starts,
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
	// FrontDoor is the front door's public address, which a server sends people to.
	FrontDoor string
	// OrgURL is the address each organization's server is reached at, which an app a person signs in to sends them back to. With
	// none, people sign in to no apps.
	OrgURL func(slug string) string
	// PortFrom is the first port organizations' servers are given, two each.
	PortFrom int
	// Wait is how long a server has to answer once it is started.
	Wait time.Duration
	// Concurrent is how many jobs a server's queue runs at once.
	Concurrent int
	// StopGrace is how long a server is given to end its jobs' records when it is
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

	// mu guards the two maps below and is held only to read or change them, so
	// routing never waits on a server that is starting. Each organization is
	// provisioned by one caller at a time, under its own lock, and ports are given
	// out under portMu, so two do not choose the same.
	mu     sync.Mutex
	states map[string]Runtime
	retry  map[string]time.Time
	// removed is when each organization Remove forgot was made, until Ended: a slug may be given again, and a new
	// organization that holds it is told apart from the removed one by when it was made.
	removed map[string]time.Time
	locks   sync.Map
	portMu  sync.Mutex

	// epoch is the lease this controller leads with, which fences what it writes: 0 when no lease governs it, and -1
	// while another front door leads, when this one routes from the database and starts nothing.
	epoch atomic.Int64
	// followed is what routing read from the database while another front door leads.
	followed routeCache
}

// Lead makes this controller the one that provisions and supervises, at the lease's epoch.
func (c *Controller) Lead(epoch int64) { c.epoch.Store(epoch) }

// Follow makes this controller route from the database and start nothing, while another front door leads. What it kept
// while it led is forgotten, since the leader changes it now.
func (c *Controller) Follow() {
	c.epoch.Store(-1)
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.states)
	clear(c.retry)
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

// Provision makes an organization's server exist and start, and says when it answers. A front door that does not lead
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
	if err := c.stillServed(ctx, slug); err != nil {
		return err
	}
	runtime, err := c.Store.Runtime(ctx, slug)
	if err != nil {
		return err
	}
	// A restart that was counted a moment ago is only in memory until it is written.
	c.mu.Lock()
	if held, known := c.states[slug]; known && held.Restarts > runtime.Restarts {
		runtime.Restarts = held.Restarts
	}
	c.mu.Unlock()
	if err := c.set(ctx, slug, runtime, Provisioning, ""); err != nil {
		return err
	}
	if err := c.prepare(ctx, slug); err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	node, err := c.placement(ctx, slug, &runtime)
	if errors.Is(err, ErrNoCapacity) {
		return c.wait(ctx, slug, runtime, err)
	}
	if err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	if runtime.Port == 0 {
		if runtime, err = c.givePorts(ctx, slug, runtime, node); err != nil {
			return c.fail(ctx, slug, runtime, err)
		}
	}
	if err := c.set(ctx, slug, runtime, Starting, ""); err != nil {
		return err
	}
	env, err := c.environment(ctx, slug, runtime, node)
	if err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	spec := ServerSpec{Slug: slug, Command: c.Command, Env: env, Log: filepath.Join(c.orgDir(slug), "server.log")}
	if err := c.endOtherRelease(node, slug, runtime); err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	if err := node.Start(spec); err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	runtime.Command = strings.Join(c.Command, " ")
	if err := c.answers(ctx, node, runtime.Port); err != nil {
		return c.fail(ctx, slug, runtime, err)
	}
	if err := c.set(ctx, slug, runtime, Ready, ""); err != nil {
		return err
	}
	// A demo's time starts when its server first answers, whoever opens it first: its visitor's waiting page, or a colleague
	// who came by the organization's own address.
	if _, err := c.Store.DemoReady(ctx, slug); err != nil && !errors.Is(err, ErrNotDemo) {
		slog.Error("a demo's time could not be started", "org", slug, "error", err)
	}
	return nil
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
	delete(c.states, slug)
	delete(c.retry, slug)
	return ErrRemoved
}

// endOtherRelease stops an organization's server that was started with another command than this front door's, so that
// it is started again as this release. A server on a host apart outlives the front door that started it, and would
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

// prepare makes what an organization's server keeps: its directories, the key it signs
// its records with, and the key it and the front door share.
func (c *Controller) prepare(ctx context.Context, slug string) error {
	data := c.Store.DataDir(slug)
	for _, dir := range []string{"jobs", "sessions", "setups", "comparisons", "changes", "retention", "keys", "computers", "inbox"} {
		if err := os.MkdirAll(filepath.Join(data, dir), 0o700); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(c.orgDir(slug), "home"), 0o700); err != nil {
		return err
	}
	if err := c.Store.PublishRoster(ctx, slug); err != nil {
		return err
	}
	keys := filepath.Join(data, "keys")
	if _, err := os.Stat(filepath.Join(keys, slug+".key")); missing(err) {
		if _, err := signing.Generate(keys, slug); err != nil {
			return err
		}
	}
	if _, err := os.Stat(c.entryKeyFile(slug)); missing(err) {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		return os.WriteFile(c.entryKeyFile(slug), []byte(hex.EncodeToString(secret)), 0o600)
	}
	return nil
}

// entryKeyFile is the key the front door makes entry tokens with and the
// organization's server checks them with.
func (c *Controller) entryKeyFile(slug string) string {
	return filepath.Join(c.orgDir(slug), "entry.key")
}

// EntryKey is that key.
func (c *Controller) EntryKey(slug string) ([]byte, error) { return os.ReadFile(c.entryKeyFile(slug)) }

// environment is the whole environment of an organization's server: the operator's
// settings, then what is the organization's own, which wins.
func (c *Controller) environment(ctx context.Context, slug string, runtime Runtime, node Node) ([]string, error) {
	password := make([]byte, 24)
	if _, err := rand.Read(password); err != nil {
		return nil, err
	}
	org, _, err := c.Store.Org(ctx, slug)
	if err != nil {
		return nil, err
	}
	own := c.dataSettings(slug)
	maps.Copy(own, map[string]string{
		"GUSTO_ADDR":       node.Listen() + ":" + strconv.Itoa(runtime.Port),
		"GUSTO_PROXY_ADDR": "127.0.0.1:" + strconv.Itoa(runtime.ProxyPort),
		"GUSTO_ORG":        slug,
		"GUSTO_ORG_NAME":   org.Name,
		"GUSTO_FRONT_DOOR": c.FrontDoor,
		"GUSTO_NODE":       node.Name(),
		// The name it had, for a server of an earlier release on a kept node.
		"GUSTO_CELL": node.Name(),
		// A person's own conversations share their computer's files and their own state, so they
		// take turns. A room's agents each work on a copy, so they do not wait for each other.
		"GUSTO_LIMIT_PERSON":    "1",
		"GUSTO_JOB_CONCURRENCY": strconv.Itoa(max(c.Concurrent, 1)),
		// Nobody knows this: a server behind the front door has no password to type.
		"GUSTO_PASSWORD": hex.EncodeToString(password),
		"HOME":           filepath.Join(c.orgDir(slug), "home"),
	})
	data := c.Store.DataDir(slug)
	if c.OrgURL != nil {
		own["GUSTO_PUBLIC_URL"] = c.OrgURL(slug)
		own["GUSTO_CONNECTORS"] = filepath.Join(data, "connectors")
	}
	operator := c.Env
	if org.Demo != nil {
		operator = append(slices.Clone(c.Env), c.DemoEnv...)
	}
	database, err := usageDatabase(operator, slug, c.indexDir())
	if err != nil {
		return nil, err
	}
	if database != "" {
		own[databaseURLEnv] = database
	}
	env := withoutSetting(operator, databaseURLEnv)
	for name, value := range own {
		env = append(env, name+"="+value)
	}
	return env, nil
}

// dataSettings is where an organization's server keeps each kind of its data, all under its own directory.
func (c *Controller) dataSettings(slug string) map[string]string {
	data := c.Store.DataDir(slug)
	return map[string]string{
		"GUSTO_JOBS":         filepath.Join(data, "jobs"),
		"GUSTO_SESSIONS":     filepath.Join(data, "sessions"),
		"GUSTO_SETUPS":       filepath.Join(data, "setups"),
		"GUSTO_CREW":         filepath.Join(data, "crew"),
		"GUSTO_PRESETS":      filepath.Join(data, "presets"),
		"GUSTO_INTEGRATIONS": filepath.Join(data, "integrations"),
		"GUSTO_COMPARISONS":  filepath.Join(data, "comparisons"),
		"GUSTO_CHANGES":      filepath.Join(data, "changes", "changes.jsonl"),
		"GUSTO_RETENTION":    filepath.Join(data, "retention"),
		signing.KeyEnv:       filepath.Join(data, "keys", slug+".key"),
		signing.KeyringEnv:   filepath.Join(data, "keys"),
		"GUSTO_SIGNER":       slug,
		"GUSTO_QUOTA":        c.Store.QuotaFile(slug),
		"GUSTO_MEMBERS":      c.Store.RosterFile(slug),
		"GUSTO_ORG_SETTINGS": filepath.Join(data, "settings.json"),
		"GUSTO_REGISTRY":     filepath.Join(data, "registry"),
		"GUSTO_INBOX":        c.Store.InboxDir(slug),
		"GUSTO_ENTRY_KEY":    c.entryKeyFile(slug),
		"GUSTO_COMPUTERS":    filepath.Join(data, "computers"),
		"GUSTO_APPS":         filepath.Join(data, "apps"),
	}
}

// givePorts gives an organization two ports on its node and writes them down before anyone
// else is given any, so two organizations provisioned together do not share one.
func (c *Controller) givePorts(ctx context.Context, slug string, runtime Runtime, node Node) (Runtime, error) {
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
	server, proxy, err := node.Ports(c.PortFrom, given)
	if err != nil {
		return runtime, err
	}
	runtime.Port, runtime.ProxyPort = server, proxy
	return runtime, c.Store.SetRuntimeAt(ctx, c.epoch.Load(), slug, runtime)
}

func free(port int) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// healthCheck bounds one ask of a server's health, so a server that takes the connection and never answers is asked again.
var healthCheck = &http.Client{Timeout: 2 * time.Second}

// answers waits until the server on the port says it is well, for as long as c.Wait or until ctx ends.
func (c *Controller) answers(ctx context.Context, node Node, port int) error {
	ctx, cancel := context.WithTimeout(ctx, c.Wait)
	defer cancel()
	address := "http://" + net.JoinHostPort(node.Host(), strconv.Itoa(port)) + "/healthz"
	every := time.NewTicker(100 * time.Millisecond)
	defer every.Stop()
	for {
		if healthy(ctx, address) {
			return nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("the server did not answer in %s, and what it said is in server.log", c.Wait)
			}
			return ctx.Err()
		case <-every.C:
		}
	}
}

// healthy is whether the server at address says it is well.
func healthy(ctx context.Context, address string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	response, err := healthCheck.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

// set writes down the state of a server and remembers it for routing. A write refused because another front door leads now
// is said, and nothing is remembered: the provision that made it stops. Any other failure to write is logged, and routing
// follows what is remembered.
func (c *Controller) set(ctx context.Context, slug string, runtime Runtime, state State, why string) error {
	runtime.State, runtime.Error = state, why
	err := c.Store.SetRuntimeAt(ctx, c.epoch.Load(), slug, runtime)
	if errors.Is(err, ErrNotLeading) {
		return err
	}
	if err != nil {
		slog.Error("an organization's state was not written", "org", slug, "error", err)
	}
	c.mu.Lock()
	if c.states == nil {
		c.states = map[string]Runtime{}
	}
	c.states[slug] = runtime
	c.mu.Unlock()
	return nil
}

// wait leaves an organization waiting for a node with room, which it is tried for again shortly. It is not a failure
// and is not counted as one, since what it waits for is being made.
func (c *Controller) wait(ctx context.Context, slug string, runtime Runtime, err error) error {
	c.set(ctx, slug, runtime, Waiting, err.Error())
	c.mu.Lock()
	if c.retry == nil {
		c.retry = map[string]time.Time{}
	}
	c.retry[slug] = time.Now().Add(c.waitEvery())
	c.mu.Unlock()
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

// Waiting is how many organizations are waiting for a node with room, which is what makes another start.
func (c *Controller) Waiting() int { return c.count(Waiting) }

// Busy is how many organizations wait for a node with room or are being placed and started, while which no node is idle:
// the one an organization is being placed on may be the one with nothing on it yet.
func (c *Controller) Busy() int { return c.count(Waiting, Provisioning, Starting) }

func (c *Controller) count(states ...State) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	counted := 0
	for _, runtime := range c.states {
		if slices.Contains(states, runtime.State) {
			counted++
		}
	}
	return counted
}

func (c *Controller) fail(ctx context.Context, slug string, runtime Runtime, err error) error {
	slog.Error("an organization's server could not be made ready", "org", slug, "error", err)
	c.set(ctx, slug, runtime, Failed, err.Error())
	return err
}

// Route is where an organization's server is when it can be reached, and its state
// when it cannot. A server whose process has gone is not reached.
func (c *Controller) Route(ctx context.Context, slug string) (address string, state State) {
	if c.following() {
		return c.routeFollowed(ctx, slug)
	}
	c.mu.Lock()
	runtime, known := c.states[slug]
	c.mu.Unlock()
	if !known {
		return "", ""
	}
	if runtime.State != Ready {
		return "", runtime.State
	}
	node, err := c.nodeNamed(runtime.Node)
	if err != nil || !node.Alive(slug) {
		return "", Starting
	}
	return net.JoinHostPort(node.Host(), strconv.Itoa(runtime.Port)), Ready
}

// Run provisions every organization there is and then keeps their servers running, until the context ends: when this front
// door stops leading, or stops. It leaves the servers running, for the next leader; StopAll stops them. A server that has
// stopped is started again, at first at once and then less often, so one that cannot start does not take the machine with
// it.
func (c *Controller) Run(ctx context.Context, every time.Duration) {
	orgs, err := c.Store.Orgs(ctx)
	if err != nil {
		slog.Error("the organizations could not be listed", "error", err)
	}
	// What each server was is known before it is started again, so a request for it
	// is told it is starting and not that it is not there.
	for _, org := range orgs {
		if runtime, err := c.Store.Runtime(ctx, org.Slug); err == nil && runtime.State != "" {
			c.set(ctx, org.Slug, runtime, Starting, "")
		}
	}
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

// supervise restarts what has stopped or failed, retries what waits, and provisions what was signed up on a front door that
// does not lead. Whether a server is alive is asked of its node outside c.mu, so routing does not wait on a node.
func (c *Controller) supervise(ctx context.Context) {
	var due, waiting []string
	var ready map[string]Runtime
	c.mu.Lock()
	now := time.Now()
	for slug, runtime := range c.states {
		switch {
		case runtime.State == Ready:
			if ready == nil {
				ready = map[string]Runtime{}
			}
			ready[slug] = runtime
		case runtime.State == Failed && !now.Before(c.retry[slug]):
			due = append(due, slug)
		case runtime.State == Waiting && !now.Before(c.retry[slug]):
			waiting = append(waiting, slug)
		}
	}
	c.mu.Unlock()
	for slug, runtime := range ready {
		if !c.alive(slug, runtime) {
			due = append(due, slug)
		}
	}
	for _, slug := range due {
		c.restart(ctx, slug)
	}
	for _, slug := range waiting {
		go c.reprovision(ctx, slug)
	}
	c.provisionNew(ctx)
}

// provisionNew provisions each organization that has no runtime yet and that this controller does not hold.
func (c *Controller) provisionNew(ctx context.Context) {
	slugs, err := c.Store.Unprovisioned(ctx)
	if err != nil {
		slog.Error("the organizations with no server could not be listed", "error", err)
		return
	}
	for _, slug := range slugs {
		c.mu.Lock()
		_, held := c.states[slug]
		c.mu.Unlock()
		if !held {
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
	c.mu.Lock()
	_, kept := c.states[slug]
	c.mu.Unlock()
	if !kept {
		return nil
	}
	return c.provision(ctx, slug)
}

// restart starts a server that has stopped or failed. One that fails again is
// tried after twice as long each time, up to five minutes.
func (c *Controller) restart(ctx context.Context, slug string) {
	c.mu.Lock()
	runtime, kept := c.states[slug]
	if !kept {
		c.mu.Unlock()
		return
	}
	runtime.Restarts++
	c.states[slug] = runtime
	c.mu.Unlock()
	if err := c.reprovision(ctx, slug); err != nil && !errors.Is(err, ErrRemoved) {
		c.mu.Lock()
		if c.retry == nil {
			c.retry = map[string]time.Time{}
		}
		c.retry[slug] = time.Now().Add(time.Duration(min(1<<min(runtime.Restarts, 8), 300)) * time.Second)
		c.mu.Unlock()
	}
}

// alive is whether an organization's server is running on its node, and a node that cannot be
// reached is one whose servers are not known to be.
func (c *Controller) alive(slug string, runtime Runtime) bool {
	node, err := c.nodeNamed(runtime.Node)
	return err == nil && node.Alive(slug)
}

// StopAll stops every server this controller holds, when the front door stops. A server on a host apart is left running:
// it does not end with the front door, which is restarted at every deploy.
func (c *Controller) StopAll() {
	c.mu.Lock()
	held := make(map[string]Runtime, len(c.states))
	for slug, runtime := range c.states {
		held[slug] = runtime
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
			// A server on another host does not end with the front door, which is restarted at every deploy, and is
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
	runtime := c.states[slug]
	delete(c.states, slug)
	delete(c.retry, slug)
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
