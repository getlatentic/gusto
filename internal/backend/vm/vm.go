// Package vm runs each turn's agent in an OpenShell sandbox, a virtual machine on a gateway. An organization's server runs a
// person's turns in a computer of their own that stays between turns, and a script's in a sandbox of its own.
package vm

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/harness"
)

// tendEvery is how often people's computers are looked over, to remove the retired and stop the idle.
const tendEvery = 30 * time.Second

// Config is how a server's sandboxes are made.
type Config struct {
	Gateway openshell.Config
	// Agents are the agents a sandbox may be asked to run.
	Agents []string
	// Computers is the record of the computers of an organization's people, or nil when every turn has a sandbox of its own.
	// The rest is read only when it is set.
	Computers *computer.Registry
	Org       string
	// Cell is where a new computer is made.
	Cell string
	// Apps are the web apps agents built, which run in their owners' computers.
	Apps *apps.Store
	// Spare keeps one blank computer ready for the next person who has none.
	Spare bool
	// Idle is how long a computer nobody uses runs before it is stopped.
	Idle time.Duration
	// ProxyPort is the proxy's port, which a spare computer's policy names.
	ProxyPort string
	// Signing is who stands behind each turn's record.
	Signing signing.Identity
}

// Backend runs turns in OpenShell sandboxes.
type Backend struct {
	cfg  Config
	use  *computerUse
	apps *sharedApps
	// spares is set by Start, before any turn runs.
	spares  *spareComputers
	once    sync.Once
	offered []catalog.Entry
}

// New is a backend of sandboxes made as cfg says.
func New(cfg Config) *Backend {
	b := &Backend{cfg: cfg, use: newComputerUse()}
	if cfg.Computers != nil {
		b.apps = newSharedApps(cfg.Gateway, cfg.Computers, cfg.Apps)
	}
	return b
}

func (b *Backend) String() string { return "in an OpenShell sandbox, " + b.cfg.Gateway.Image }

// Check says why this computer cannot make the gateway's virtual machines.
func (b *Backend) Check() error { return openshell.HostCannotMakeVMs(b.cfg.Gateway.Remote) }

// Reach is the loopback: a sandbox reaches this computer's loopback through the gateway's own proxy.
func (b *Backend) Reach() backend.Reach { return backend.Loopback }

// Offered is what a sandbox of the image carries, asked of one sandbox and remembered beside the image.
func (b *Backend) Offered() []catalog.Entry {
	b.once.Do(func() {
		b.offered = catalog.SurveyOf(openshell.Remembered(openshell.Image(b.cfg.Gateway), b.cfg.Gateway.Image), b.cfg.Agents)
	})
	return b.offered
}

// Apps runs the web apps agents built in their owners' computers. It is nil when there are no computers to run them in.
func (b *Backend) Apps() apps.Runner {
	if b.apps == nil {
		return nil
	}
	return b.apps
}

// Start looks after people's computers until ctx ends: it removes the computers of people removed from the organization,
// stops the ones nobody has used for a while, and keeps a spare ready when Spare is set.
func (b *Backend) Start(ctx context.Context) (func(), error) {
	if b.cfg.Computers == nil {
		return func() {}, nil
	}
	tending, stop := context.WithCancel(ctx)
	if b.cfg.Spare {
		spares, err := openSpareComputers(tending, b.cfg.Gateway, b.cfg.Org, "http://"+b.cfg.Gateway.Host+":"+b.cfg.ProxyPort+"/model/v1")
		if err != nil {
			stop()
			return nil, err
		}
		b.spares = spares
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		tendComputers(tending, b.cfg.Gateway, b.cfg.Computers, b.use, b.apps.serving, tendEvery, b.cfg.Idle)
	}()
	return func() { stop(); <-done; b.spares.close() }, nil
}

// Run runs a turn in the computer of the person it counts against, and one asked for by a script or a token, which is
// nobody's, in a sandbox of its own. A sandbox has its own files, so a conversation's workspace is copied into the turn's before
// it and back after it. It is copied back only when the turn got as far as writing its record, which is after its workspace
// came out of the sandbox, so a turn that failed on the way leaves the instance as it was.
func (b *Backend) Run(ctx context.Context, turn backend.Turn) (int, error) {
	if turn.Exit.Door == nil {
		return 0, runner.Refuse(turn.Dir, errors.New("a sandbox on a gateway reaches its model through the proxy, and this deployment holds no credential for one"))
	}
	if turn.Workspace != "" {
		if err := replaceTree(turn.Workspace, turn.Dir.Workspace()); err != nil {
			return 0, err
		}
	}
	exit, err := b.ran(ctx, turn)
	if _, read := evidence.Read(turn.Dir); read != nil || turn.Workspace == "" {
		return exit, err
	}
	if back := replaceTree(turn.Dir.Workspace(), turn.Workspace); back != nil && err == nil {
		err = back
	}
	return exit, err
}

func (b *Backend) ran(ctx context.Context, turn backend.Turn) (int, error) {
	if b.cfg.Computers != nil && turn.Person != "" {
		return b.inComputer(ctx, turn)
	}
	return b.inSandbox(ctx, turn)
}

// inSandbox runs a turn's agent in a sandbox of its own. The proxy is reached on the name the sandbox gives this machine, and
// the runner stays here and writes the record.
func (b *Backend) inSandbox(ctx context.Context, turn backend.Turn) (int, error) {
	dir, spec, door := turn.Dir, turn.Spec, turn.Exit.Door
	token, bound, err := door.SandboxToken(dir, spec)
	if err != nil {
		return 0, err
	}
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return 0, err
	}
	gw, port, id := b.cfg.Gateway, turn.Exit.Port, filepath.Base(dir.Path())
	box, err := openshell.New(gw, id, configured.Adapter, "http://"+gw.Host+":"+port+"/model/v1", "http://127.0.0.1:"+port+"/model/v1", token)
	if err != nil {
		return 0, err
	}
	if bound {
		box.Bind()
	}
	if err := door.RecordRun(dir, spec, openshell.Name(id), bound); err != nil {
		return 0, err
	}
	return runner.Outside(ctx, dir, box, b.cfg.Signing)
}
