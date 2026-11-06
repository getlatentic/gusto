package vm

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/harness"
)

// inComputer runs a turn in the computer of the person it counts against, making it when they have none. What is refused, a
// computer past the organization's quota among it, is said in the turn's record like any other refusal.
func (b *Backend) inComputer(ctx context.Context, turn backend.Turn) (int, error) {
	dir, spec, door := turn.Dir, turn.Spec, turn.Exit.Door
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return 0, err
	}
	gw, port := b.cfg.Gateway, turn.Exit.Port
	modelURL := "http://" + gw.Host + ":" + port + "/model/v1"
	held, _, err := b.cfg.Computers.EnsureFrom(turn.Person, b.cfg.Cell, b.spares.offerFor(gw, modelURL, configured.Adapter))
	if err != nil {
		return 0, runner.Refuse(dir, err)
	}
	token, bound, err := door.SandboxToken(dir, spec)
	if err != nil {
		return 0, failedBeforeStart(b.cfg.Computers, held.ID, err)
	}
	box, err := openshell.NewComputer(gw, held.Sandbox, filepath.Base(dir.Path()), folderInComputer(turn), configured.Adapter,
		modelURL, "http://127.0.0.1:"+port+"/model/v1", token)
	if err != nil {
		return 0, failedBeforeStart(b.cfg.Computers, held.ID, err)
	}
	if bound {
		box.Bind()
	}
	if err := door.RecordRun(dir, spec, held.Sandbox, bound); err != nil {
		return 0, failedBeforeStart(b.cfg.Computers, held.ID, err)
	}
	// A room's turn sees only the room: its copy of the folder and its agent's own memory, not the rest of the person's
	// computer (getlatentic/planning#731).
	if turn.InRoom() {
		box.Confine(folderInComputer(turn))
	}
	return runner.Outside(ctx, dir, &computerBox{Sandbox: box, registry: b.cfg.Computers, id: held.ID, use: b.use}, b.cfg.Signing)
}

// failedBeforeStart says why a computer being made never started, so it is not left being made for good, counting against
// the quota. A computer that had started before keeps its state: this task failed, not it.
func failedBeforeStart(registry *computer.Registry, id string, err error) error {
	if _, changeErr := registry.Change(id, func(each *computer.Computer) {
		if each.State == computer.Creating {
			each.State, each.Note = computer.Failed, err.Error()
		}
	}); changeErr != nil {
		slog.Error("a workspace's record was not changed", "workspace", id, "error", changeErr)
	}
	return err
}

// folderInComputer names the folder a turn works in in the person's computer. A room's agents run at the same time, each on
// its own copy of the room's folder, and two of them asked by one person run in that person's one computer, so each agent of a
// room has a folder of its own there; its own turns go one at a time. Any other conversation has its instance's.
func folderInComputer(turn backend.Turn) string {
	if turn.InRoom() {
		return turn.Spec.Session + "-" + turn.Spec.Agent
	}
	return turn.Spec.Session
}

// computerBox is a computer for a task that says in the record what became of it.
type computerBox struct {
	*openshell.Sandbox
	registry *computer.Registry
	id       string
	// use counts this task in the computer from before its sandbox starts until it closes, so the idle stop leaves it.
	use     *computerUse
	entered bool
}

func (c *computerBox) Start(ctx context.Context, workspace string) error {
	if c.use != nil {
		c.use.enter(c.id)
		c.entered = true
	}
	if err := c.Sandbox.Start(ctx, workspace); err != nil {
		c.change(computer.Failed, err.Error())
		return err
	}
	c.change(computer.Running, "")
	return nil
}

// Close notes when the computer was last used. A computer that failed to start is still
// failed, and says why, and is not made running by a task's ending.
func (c *computerBox) Close() error {
	err := c.Sandbox.Close()
	if c.entered {
		c.use.leave(c.id)
		c.entered = false
	}
	c.update(func(each *computer.Computer) {
		each.LastActive = now()
		if each.State != computer.Failed {
			each.State = computer.Running
		}
	})
	return err
}

func (c *computerBox) change(state computer.State, note string) {
	c.update(func(each *computer.Computer) { each.State, each.Note, each.LastActive = state, note, now() })
}

// update changes the record of the computer a task used. A computer removed while the task used it stays removed: the task
// starting or ending does not bring its record back for a sandbox that is gone.
func (c *computerBox) update(change func(*computer.Computer)) {
	_, err := c.registry.Change(c.id, func(each *computer.Computer) {
		if each.State != computer.Deleted {
			change(each)
		}
	})
	if err != nil {
		slog.Error("a workspace's record was not changed", "workspace", c.id, "error", err)
	}
}

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }

// tendComputers removes the computers people were removed from the organization
// for, and stops those nobody has used for a while. Removing needs the gateway, which
// this server reaches and gusto cloud does not, so gusto cloud only marks them.
func tendComputers(ctx context.Context, cfg openshell.Config, registry *computer.Registry, use *computerUse, serving func(person string) bool, every, idle time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
		retire(cfg, registry)
		rest(cfg, registry, use, serving, idle)
	}
}

func retire(cfg openshell.Config, registry *computer.Registry) {
	retiring, err := registry.Retiring()
	if err != nil {
		slog.Error("the workspaces to remove could not be listed", "error", err)
		return
	}
	for _, each := range retiring {
		if err := openshell.Delete(cfg, each.Sandbox); err != nil {
			slog.Error("a workspace could not be removed", "workspace", each.ID, "error", err)
			continue
		}
		if err := registry.Retired(each.ID); err != nil {
			slog.Error("a removed workspace's record was not changed", "workspace", each.ID, "error", err)
		}
	}
}

// rest stops the computers nobody has used for a while: none of their tasks is running, under the computer's lock, and none
// serves a shared app.
func rest(cfg openshell.Config, registry *computer.Registry, use *computerUse, serving func(string) bool, idle time.Duration) {
	all, err := registry.All()
	if err != nil {
		return
	}
	for _, each := range all {
		if each.State != computer.Running || each.LastActive.IsZero() || time.Since(each.LastActive) < idle || serving(each.User) {
			continue
		}
		use.stopIdle(each.ID, func() {
			if err := openshell.Stop(cfg, each.Sandbox); err != nil {
				slog.Error("an idle workspace could not be stopped", "workspace", each.ID, "error", err)
				return
			}
			registry.Change(each.ID, func(c *computer.Computer) { c.State, c.Note = computer.Stopped, "stopped when idle" })
		})
	}
}
