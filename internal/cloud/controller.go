package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/signing"
)

// Controller makes each organization's server exist and keeps it running. An
// organization is provisioned when it is made and again when the front door starts,
// by steps that are each safe to run again, and the state of each is written down as
// it goes, so a crash between two steps leaves something the next pass finishes.
type Controller struct {
	Store *Store
	// Cell is the one place servers run, when Cells is not set.
	Cell Cell
	// Cells is where servers may run when there is more than one. An organization is placed on
	// one when it is made and stays there, because its computers are on that cell's gateway.
	Cells CellSet
	// Command is the program that is an organization's server, and its arguments.
	Command []string
	// Env is the settings every organization's server has, and only those and
	// its own: the operator's credentials directory, models and sandbox.
	Env []string
	// FrontDoor is the front door's public address, which a server sends people to.
	FrontDoor string
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
	// WaitEvery is how often an organization waiting for a cell is tried again. It is half a second when not set: a cell
	// says it is ready by writing to the shared file system, which cannot tell a reader it changed, and a visitor waits
	// on the organization.
	WaitEvery time.Duration
	// OnWaiting is told when an organization starts waiting for a cell with room, so a cell can be started for it at
	// once. It is called outside the controller's lock and must not block.
	OnWaiting func()

	// mu guards the two maps below and is held only to read or change them, so
	// routing never waits on a server that is starting. Each organization is
	// provisioned by one caller at a time, under its own lock, and ports are given
	// out under portMu, so two do not choose the same.
	mu     sync.Mutex
	states map[string]Runtime
	retry  map[string]time.Time
	locks  sync.Map
	portMu sync.Mutex
}

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

// Provision makes an organization's server exist and start, and says when it answers.
func (c *Controller) Provision(slug string) error {
	held := c.lock(slug)
	held.Lock()
	defer held.Unlock()
	return c.provision(slug)
}

func (c *Controller) provision(slug string) error {
	runtime, err := c.Store.Runtime(slug)
	if err != nil {
		return err
	}
	// A restart that was counted a moment ago is only in memory until it is written.
	c.mu.Lock()
	if held, known := c.states[slug]; known && held.Restarts > runtime.Restarts {
		runtime.Restarts = held.Restarts
	}
	c.mu.Unlock()
	c.set(slug, runtime, Provisioning, "")
	if err := c.prepare(slug); err != nil {
		return c.fail(slug, runtime, err)
	}
	cell, err := c.placement(slug, &runtime)
	if errors.Is(err, ErrNoCapacity) {
		return c.wait(slug, runtime, err)
	}
	if err != nil {
		return c.fail(slug, runtime, err)
	}
	if runtime.Port == 0 {
		if runtime, err = c.givePorts(slug, runtime, cell); err != nil {
			return c.fail(slug, runtime, err)
		}
	}
	c.set(slug, runtime, Starting, "")
	env, err := c.environment(slug, runtime, cell)
	if err != nil {
		return c.fail(slug, runtime, err)
	}
	spec := ServerSpec{Slug: slug, Command: c.Command, Env: env, Log: filepath.Join(c.orgDir(slug), "server.log")}
	if err := cell.Start(spec); err != nil {
		return c.fail(slug, runtime, err)
	}
	if err := c.answers(cell, runtime.Port); err != nil {
		return c.fail(slug, runtime, err)
	}
	c.set(slug, runtime, Ready, "")
	return nil
}

// cellNamed is the cell an organization was placed on.
func (c *Controller) cellNamed(name string) (Cell, error) {
	if c.Cells == nil {
		return c.Cell, nil
	}
	return c.Cells.Get(name)
}

// placement is the cell an organization's server runs on. One that was placed stays there, since its
// computers are on that cell's gateway, and one that was not is put on the cell with the most room and
// has that written down before anything is started on it.
func (c *Controller) placement(slug string, runtime *Runtime) (Cell, error) {
	if runtime.Cell != "" {
		cell, err := c.cellNamed(runtime.Cell)
		if !errors.Is(err, ErrCellGone) {
			return cell, err
		}
		slog.Warn("an organization's host is gone, so it is placed on another", "org", slug, "cell", runtime.Cell)
		// The ports were the gone cell's, and are not this one's.
		runtime.Cell, runtime.Port, runtime.ProxyPort = "", 0, 0
	}
	cell := c.Cell
	if c.Cells != nil {
		var err error
		if cell, err = c.Cells.Place(); err != nil {
			return nil, err
		}
	}
	runtime.Cell = cell.Name()
	return cell, c.Store.SetRuntime(slug, *runtime)
}

// prepare makes what an organization's server keeps: its directories, the key it signs
// its records with, and the key it and the front door share.
func (c *Controller) prepare(slug string) error {
	data := c.Store.DataDir(slug)
	for _, dir := range []string{"jobs", "sessions", "setups", "comparisons", "changes", "retention", "keys", "computers", "inbox"} {
		if err := os.MkdirAll(filepath.Join(data, dir), 0o700); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(c.orgDir(slug), "home"), 0o700); err != nil {
		return err
	}
	if err := c.Store.PublishRoster(slug); err != nil {
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
func (c *Controller) environment(slug string, runtime Runtime, cell Cell) ([]string, error) {
	password := make([]byte, 24)
	if _, err := rand.Read(password); err != nil {
		return nil, err
	}
	data := c.Store.DataDir(slug)
	own := map[string]string{
		"GUSTO_ADDR":         cell.Listen() + ":" + strconv.Itoa(runtime.Port),
		"GUSTO_PROXY_ADDR":   "127.0.0.1:" + strconv.Itoa(runtime.ProxyPort),
		"GUSTO_JOBS":         filepath.Join(data, "jobs"),
		"GUSTO_SESSIONS":     filepath.Join(data, "sessions"),
		"GUSTO_SETUPS":       filepath.Join(data, "setups"),
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
		"GUSTO_ORG":          slug,
		"GUSTO_FRONT_DOOR":   c.FrontDoor,
		"GUSTO_COMPUTERS":    filepath.Join(data, "computers"),
		"GUSTO_APPS":         filepath.Join(data, "apps"),
		"GUSTO_CELL":         cell.Name(),
		// A person's harnesses share their computer's files and their own state, so they
		// take turns.
		"GUSTO_LIMIT_PERSON":    "1",
		"GUSTO_JOB_CONCURRENCY": strconv.Itoa(max(c.Concurrent, 1)),
		// Nobody knows this: a server behind the front door has no password to type.
		"GUSTO_PASSWORD": hex.EncodeToString(password),
		"HOME":           filepath.Join(c.orgDir(slug), "home"),
	}
	database, err := usageDatabase(c.Env, slug, c.indexDir())
	if err != nil {
		return nil, err
	}
	if database != "" {
		own[databaseURLEnv] = database
	}
	env := withoutSetting(c.Env, databaseURLEnv)
	for name, value := range own {
		env = append(env, name+"="+value)
	}
	return env, nil
}

// givePorts gives an organization two ports on its cell and writes them down before anyone
// else is given any, so two organizations provisioned together do not share one.
func (c *Controller) givePorts(slug string, runtime Runtime, cell Cell) (Runtime, error) {
	c.portMu.Lock()
	defer c.portMu.Unlock()
	orgs, err := c.Store.Orgs()
	if err != nil {
		return runtime, err
	}
	given := map[int]bool{}
	for _, org := range orgs {
		if other, err := c.Store.Runtime(org.Slug); err == nil && other.Cell == runtime.Cell {
			given[other.Port], given[other.ProxyPort] = true, true
		}
	}
	server, proxy, err := cell.Ports(c.PortFrom, given)
	if err != nil {
		return runtime, err
	}
	runtime.Port, runtime.ProxyPort = server, proxy
	return runtime, c.Store.SetRuntime(slug, runtime)
}

func free(port int) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// answers waits until the server on the port says it is well.
func (c *Controller) answers(cell Cell, port int) error {
	deadline := time.Now().Add(c.Wait)
	for {
		if response, err := http.Get("http://" + net.JoinHostPort(cell.Host(), strconv.Itoa(port)) + "/healthz"); err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the server did not answer in %s, and what it said is in server.log", c.Wait)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// set writes down the state of a server and remembers it for routing.
func (c *Controller) set(slug string, runtime Runtime, state State, why string) {
	runtime.State, runtime.Error = state, why
	c.mu.Lock()
	if c.states == nil {
		c.states = map[string]Runtime{}
	}
	c.states[slug] = runtime
	c.mu.Unlock()
	if err := c.Store.SetRuntime(slug, runtime); err != nil {
		slog.Error("an organization's state was not written", "org", slug, "error", err)
	}
}

// wait leaves an organization waiting for a cell with room, which it is tried for again shortly. It is not a failure
// and is not counted as one, since what it waits for is being made.
func (c *Controller) wait(slug string, runtime Runtime, err error) error {
	c.set(slug, runtime, Waiting, err.Error())
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

// Waiting is how many organizations are waiting for a cell with room, which is what makes another start.
func (c *Controller) Waiting() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	waiting := 0
	for _, runtime := range c.states {
		if runtime.State == Waiting {
			waiting++
		}
	}
	return waiting
}

func (c *Controller) fail(slug string, runtime Runtime, err error) error {
	slog.Error("an organization's server could not be made ready", "org", slug, "error", err)
	c.set(slug, runtime, Failed, err.Error())
	return err
}

// Route is where an organization's server is when it can be reached, and its state
// when it cannot. A server whose process has gone is not reached.
func (c *Controller) Route(slug string) (address string, state State) {
	c.mu.Lock()
	runtime, known := c.states[slug]
	c.mu.Unlock()
	if !known {
		return "", ""
	}
	if runtime.State != Ready {
		return "", runtime.State
	}
	cell, err := c.cellNamed(runtime.Cell)
	if err != nil || !cell.Alive(slug) {
		return "", Starting
	}
	return net.JoinHostPort(cell.Host(), strconv.Itoa(runtime.Port)), Ready
}

// Run provisions every organization there is and then keeps their servers running,
// until the context ends, when it stops them all. A server that has stopped is
// started again, at first at once and then less often, so one that cannot start
// does not take the machine with it.
func (c *Controller) Run(ctx context.Context, every time.Duration) {
	orgs, err := c.Store.Orgs()
	if err != nil {
		slog.Error("the organizations could not be listed", "error", err)
	}
	// What each server was is known before it is started again, so a request for it
	// is told it is starting and not that it is not there.
	for _, org := range orgs {
		if runtime, err := c.Store.Runtime(org.Slug); err == nil && runtime.State != "" {
			c.set(org.Slug, runtime, Starting, "")
		}
	}
	for _, org := range orgs {
		go c.Provision(org.Slug)
	}
	for {
		// An organization that waits for a cell is looked at often, since a visitor waits on it; otherwise every so often.
		next := every
		if c.Waiting() > 0 {
			next = min(every, c.waitEvery())
		}
		timer := time.NewTimer(next)
		select {
		case <-ctx.Done():
			timer.Stop()
			c.stopAll()
			return
		case <-timer.C:
			c.supervise()
		}
	}
}

func (c *Controller) supervise() {
	var due, waiting []string
	c.mu.Lock()
	now := time.Now()
	for slug, runtime := range c.states {
		switch {
		case runtime.State == Ready && !c.alive(slug, runtime):
			due = append(due, slug)
		case runtime.State == Failed && !now.Before(c.retry[slug]):
			due = append(due, slug)
		case runtime.State == Waiting && !now.Before(c.retry[slug]):
			waiting = append(waiting, slug)
		}
	}
	c.mu.Unlock()
	for _, slug := range due {
		c.restart(slug)
	}
	for _, slug := range waiting {
		go c.Provision(slug)
	}
}

// restart starts a server that has stopped or failed. One that fails again is
// tried after twice as long each time, up to five minutes.
func (c *Controller) restart(slug string) {
	c.mu.Lock()
	runtime := c.states[slug]
	runtime.Restarts++
	c.states[slug] = runtime
	c.mu.Unlock()
	if err := c.Provision(slug); err != nil {
		c.mu.Lock()
		if c.retry == nil {
			c.retry = map[string]time.Time{}
		}
		c.retry[slug] = time.Now().Add(time.Duration(min(1<<min(runtime.Restarts, 8), 300)) * time.Second)
		c.mu.Unlock()
	}
}

// alive is whether an organization's server is running on its cell, and a cell that cannot be
// reached is one whose servers are not known to be.
func (c *Controller) alive(slug string, runtime Runtime) bool {
	cell, err := c.cellNamed(runtime.Cell)
	return err == nil && cell.Alive(slug)
}

func (c *Controller) stopAll() {
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
			cell, err := c.cellNamed(runtime.Cell)
			if err != nil {
				return
			}
			// A server on another host does not end with the front door, which is restarted at every deploy, and is
			// found running by the one that starts.
			if apart, ok := cell.(interface{ Detached() bool }); ok && apart.Detached() {
				return
			}
			cell.Stop(slug, c.grace())
		}()
	}
	wg.Wait()
}

// Remove stops an organization's server and forgets it, so it is not started again. What the organization's
// directory holds is the store's to delete, after this.
func (c *Controller) Remove(slug string) {
	held := c.lock(slug)
	held.Lock()
	defer held.Unlock()
	c.mu.Lock()
	runtime := c.states[slug]
	delete(c.states, slug)
	delete(c.retry, slug)
	c.mu.Unlock()
	if runtime.Cell == "" {
		if stored, err := c.Store.Runtime(slug); err == nil {
			runtime = stored
		}
	}
	if cell, err := c.cellNamed(runtime.Cell); err == nil {
		cell.Stop(slug, c.grace())
	}
}

func (c *Controller) grace() time.Duration {
	if c.StopGrace > 0 {
		return c.StopGrace
	}
	return 30 * time.Second
}
