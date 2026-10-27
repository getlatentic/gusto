package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// computersFromEnv is the record of the computers people in this organization have, when
// this server is an organization's and runs tasks in sandboxes: GUSTO_COMPUTERS is its
// directory, GUSTO_ORG the organization, and GUSTO_QUOTA is the limit on how many.
func computersFromEnv() (*computer.Registry, error) {
	dir, org := os.Getenv("GUSTO_COMPUTERS"), os.Getenv("GUSTO_ORG")
	if dir == "" || os.Getenv("GUSTO_SANDBOX") != "openshell" {
		return nil, nil
	}
	if org == "" {
		return nil, errors.New("GUSTO_COMPUTERS names the workspaces of an organization, and GUSTO_ORG says which")
	}
	registry, err := computer.Open(dir, org)
	if err != nil {
		return nil, err
	}
	if path := os.Getenv("GUSTO_QUOTA"); path != "" {
		registry.UseQuota(quota.Open(path))
	}
	return registry, nil
}

// personWhoAsked is the address of the person a task was asked for by, or nothing when
// it was asked by a token or a script, which have no computer of their own.
func personWhoAsked(index *jobs.Index, dir job.Dir) string {
	record, found := index.Get(filepath.Base(dir.Path()))
	if !found || !strings.Contains(record.Asked, "@") || strings.HasPrefix(record.Asked, "token:") {
		return ""
	}
	return strings.ToLower(record.Asked)
}

// RunInComputer runs a task in the computer of the person who asked for it, making it
// when they have none. What is refused, a computer past the organization's quota
// among it, is said in the task's record like any other refusal.
func (w *WayOut) RunInComputer(ctx context.Context, dir job.Dir, spec job.Spec, cfg openshell.Config, registry *computer.Registry, person string) (int, error) {
	if w.proxy == nil {
		return 0, runner.Refuse(dir, errors.New("a sandbox on a gateway reaches its model through the proxy, and this deployment holds no credential for one"))
	}
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return 0, err
	}
	port := w.proxyPort()
	modelURL := "http://" + cfg.Host + ":" + port + "/model/v1"
	held, _, err := registry.EnsureFrom(person, setting("GUSTO_CELL", "local"), w.spares.offerFor(cfg, modelURL, configured.Adapter))
	if err != nil {
		return 0, runner.Refuse(dir, err)
	}
	token, err := w.admit(dir, spec)
	if err != nil {
		return 0, err
	}
	box, err := openshell.NewComputer(cfg, held.Sandbox, filepath.Base(dir.Path()), folderInComputer(spec), configured.Adapter,
		modelURL, "http://127.0.0.1:"+port+"/model/v1", token)
	if err != nil {
		return 0, err
	}
	// A room's turn sees only the room: its copy of the folder and its agent's own memory, not the rest of the person's
	// computer (getlatentic/planning#731).
	if isRoomTurn(spec) {
		box.Confine(folderInComputer(spec))
	}
	return runner.OutsideContext(ctx, dir, &computerBox{Sandbox: box, registry: registry, id: held.ID})
}

// folderInComputer names the folder a turn works in in the person's computer. A room's agents run at the same time, each on
// its own copy of the room's folder, and two of them asked by one person run in that person's one computer, so each agent of a
// room has a folder of its own there; its own turns go one at a time. Any other conversation has its instance's.
func folderInComputer(spec job.Spec) string {
	if isRoomTurn(spec) {
		return spec.Session + "-" + spec.Agent
	}
	return spec.Session
}

// computerBox is a computer for a task that says in the record what became of it.
type computerBox struct {
	*openshell.Sandbox
	registry *computer.Registry
	id       string
}

func (c *computerBox) Start(ctx context.Context, workspace string) error {
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

func (c *computerBox) update(change func(*computer.Computer)) {
	if _, err := c.registry.Change(c.id, change); err != nil {
		slog.Error("a workspace's record was not changed", "workspace", c.id, "error", err)
	}
}

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }

// tendComputers removes the computers people were removed from the organization
// for, and stops those nobody has used for a while. Removing needs the gateway, which
// this server reaches and gusto cloud does not, so gusto cloud only marks them.
func tendComputers(ctx context.Context, cfg openshell.Config, registry *computer.Registry, busy func(person string) bool, every, idle time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
		retire(cfg, registry)
		rest(cfg, registry, busy, idle)
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

func rest(cfg openshell.Config, registry *computer.Registry, busy func(string) bool, idle time.Duration) {
	all, err := registry.All()
	if err != nil {
		return
	}
	for _, each := range all {
		if each.State != computer.Running || each.LastActive.IsZero() || time.Since(each.LastActive) < idle || busy(each.User) {
			continue
		}
		if err := openshell.Stop(cfg, each.Sandbox); err != nil {
			slog.Error("an idle workspace could not be stopped", "workspace", each.ID, "error", err)
			continue
		}
		registry.Change(each.ID, func(c *computer.Computer) { c.State, c.Note = computer.Stopped, "stopped when idle" })
	}
}

// running says a person has a task running now, which keeps their computer awake.
func running(index *jobs.Index, person string) bool {
	for _, record := range index.List() {
		if record.State == jobs.Running && strings.EqualFold(record.Asked, person) {
			return true
		}
	}
	return false
}
