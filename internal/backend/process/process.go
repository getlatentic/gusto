// Package process runs each turn's agent in the server's own process, which is what a person's own computer has until it has a
// box to give a turn: behind this computer's own sandbox when it has one, or behind nothing.
package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/task"
)

// Config is how turns are run in this process.
type Config struct {
	// Agents are the agents a turn may be asked to run.
	Agents []string
	// Walled puts each turn behind this computer's own sandbox.
	Walled bool
	// Open leaves the network open to a walled turn. Otherwise it reaches nothing but the proxy.
	Open bool
	// Signing is who stands behind each turn's record.
	Signing signing.Identity
}

// Backend runs turns in this process.
type Backend struct {
	cfg Config
}

// New is a backend that runs turns in this process as cfg says.
func New(cfg Config) *Backend { return &Backend{cfg: cfg} }

func (b *Backend) String() string {
	if b.cfg.Walled {
		return "here, behind this computer's own sandbox"
	}
	return "here, in this process"
}

// Check says why this computer cannot hold a turn behind its own sandbox, which is tried with a real process. A turn behind
// nothing can always run.
func (b *Backend) Check() error {
	if !b.cfg.Walled {
		return nil
	}
	set := isolation.Settings{}
	if b.cfg.Open {
		set.TaskNetwork = "open"
	}
	wall := isolation.ProcessWall(isolation.This(set))
	if wall.Ready {
		return nil
	}
	if wall.Cause != "" {
		return errors.New(wall.Detail + ": " + wall.Cause)
	}
	return errors.New(wall.Detail)
}

// Reach is the loopback: a turn here is on the proxy's own computer.
func (b *Backend) Reach() backend.Reach { return backend.Loopback }

// Start has nothing to keep up.
func (b *Backend) Start(context.Context) (func(), error) { return func() {}, nil }

// Offered is the agents whose programs are on this computer's PATH now. A person installs an agent while the server runs, and
// looking at the PATH costs nothing, so it is looked at each time.
func (b *Backend) Offered() []catalog.Entry { return catalog.SurveyOf(catalog.Machine(), b.cfg.Agents) }

// Run runs the turn's agent here. A turn in a container gets the instance's workspace and state mounted over its own; here the
// task's workspace is made a link to the instance's, and the agent is given a home of the task's own with the instance's state
// as its data, so a conversation works the same way in both places.
func (b *Backend) Run(ctx context.Context, turn backend.Turn) (int, error) {
	if turn.Workspace != "" {
		if err := pointAt(turn.Workspace, turn.Dir); err != nil {
			return 0, err
		}
	}
	home, data, err := taskHome(turn)
	if err != nil {
		return 0, err
	}
	env, err := agentEnv(turn)
	if err != nil {
		return 0, err
	}
	env.Home, env.Data = home, data
	env.Base = homedEnv(env.Base, home, data, sharedCache())
	if b.cfg.Walled {
		if env.Wrap, err = behindProcessWall(turn.Dir, turn.Spec, home, data, turn.Exit.Port, b.cfg.Open); err != nil {
			return 0, err
		}
	}
	return runner.InsideWith(ctx, turn.Dir, env, runner.Place{Signing: b.cfg.Signing})
}

// taskHome is the home an agent run here is given, as a container's /root is: a folder of the task's own, holding each agent's
// configuration as the image does, and the instance's state as its data, which is what a container mounts at /root/.local/share.
func taskHome(turn backend.Turn) (home, data string, err error) {
	if home, err = filepath.Abs(filepath.Join(turn.Dir.Path(), "home")); err != nil {
		return "", "", err
	}
	data = filepath.Join(home, ".local", "share")
	if turn.State != "" {
		if data, err = filepath.Abs(turn.State); err != nil {
			return "", "", err
		}
	}
	for _, folder := range []string{home, data} {
		if err := os.MkdirAll(folder, 0o700); err != nil {
			return "", "", err
		}
	}
	return home, data, harness.PlaceConfig(home)
}

// agentEnv is what an agent run in the server's own process is started with: the proxy, on this machine's loopback, and the
// token that stands in for the turn's credential, so its calls are capped and recorded as a box's are, and of the server's
// environment only what a program needs to run. Without a proxy the agent reaches the model it was configured with directly.
func agentEnv(turn backend.Turn) (harness.Env, error) {
	url, token := turn.Exit.Direct, ""
	if door := turn.Exit.Door; door != nil {
		minted, err := door.Token(turn.Dir, turn.Spec)
		if err != nil {
			return harness.Env{}, err
		}
		url, token = "http://127.0.0.1:"+turn.Exit.Port+"/model/v1", minted
	}
	base := append(plainEnv(), "GUSTO_MODEL_URL="+url)
	if token != "" {
		base = append(base, "GUSTO_MODEL_TOKEN="+token)
	}
	return harness.Env{Base: base, ModelURL: url, ModelToken: token}, nil
}

// pointAt makes a task's workspace a link to the instance's.
//
// The task directory is made before it is queued, which leaves a real, empty directory where the workspace goes; a link cannot
// be put over one. Remove refuses a directory with anything in it, so a task that has its own checkout is never quietly
// replaced.
func pointAt(workspace string, dir task.Dir) error {
	if err := dir.Make(); err != nil {
		return err
	}
	if err := os.Remove(dir.Workspace()); err != nil && !os.IsNotExist(err) {
		return err
	}
	// A link's relative target is read from the link's own folder, not from where the server runs.
	target, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	return linkDir(target, dir.Workspace())
}
