package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/roomwork"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/job"
)

// The agents of a room run at the same time. Each turn works on a copy of the room's folder and keeps its memory in a folder
// of its own; what it changed is merged into the room's folder when it passes. See docs/ROOM-WORK.md.

// agentStateLimits bound the copy that seeds an agent's own state from the room's.
var agentStateLimits = roomwork.Limits{Bytes: 8 << 30, Files: 1 << 20}

// roomRuns is how turns of a room's agents get their working copies.
type roomRuns struct {
	instances *instance.Store
	hub       *roomwork.Hub
	// askedBy is the person a turn was asked for by, or nothing for a token or a script.
	askedBy func(job.Dir) string
}

func newRoomRuns(instances *instance.Store, askedBy func(job.Dir) string) *roomRuns {
	if askedBy == nil {
		askedBy = func(job.Dir) string { return "" }
	}
	return &roomRuns{instances: instances, askedBy: askedBy, hub: roomwork.NewHub(roomwork.Limits{
		Bytes: int64(number("GUSTO_ROOM_COPY_MB", 4096)) << 20,
		Files: number("GUSTO_ROOM_COPY_FILES", 200000),
	})}
}

// isRoomTurn says whether a turn is one of a room's agents. Only a room's turns name an agent.
func isRoomTurn(spec job.Spec) bool { return spec.Session != "" && spec.Agent != "" }

// stateDir is where the agent keeps its memory of the conversation. A room's agent has a folder of its own, because the room's
// agents run at the same time; any other conversation shares its instance's.
func stateDir(instances *instance.Store, spec job.Spec) string {
	if isRoomTurn(spec) {
		return instances.AgentState(spec.Session, spec.Agent)
	}
	return instances.State(spec.Session)
}

// around runs a turn. A turn of a room's agent runs on a copy of the room's folder, and when it passed, what it changed is
// merged into the folder. A turn that failed, or was stopped, changes nothing there. Any other turn is simply run.
func (r *roomRuns) around(ctx context.Context, spec job.Spec, dir job.Dir, run func() (int, error)) (int, error) {
	if !isRoomTurn(spec) {
		return run()
	}
	turn, err := r.begin(ctx, spec, dir)
	if err != nil {
		return 0, err
	}
	defer turn.Close()
	exit, err := run()
	if err != nil || exit != 0 || ctx.Err() != nil {
		return exit, err
	}
	return exit, r.bring(ctx, turn, spec, dir)
}

// begin makes the turn's copy of the room's folder and its agent's own state. A turn that cannot is refused with the reason in
// its record, as a task is that the runner refuses, and one that was stopped on the way is not.
func (r *roomRuns) begin(ctx context.Context, spec job.Spec, dir job.Dir) (*roomwork.Turn, error) {
	turn, err := r.copy(ctx, spec, dir)
	if err != nil && ctx.Err() == nil {
		err = runner.Refuse(dir, err)
	}
	return turn, err
}

func (r *roomRuns) copy(ctx context.Context, spec job.Spec, dir job.Dir) (*roomwork.Turn, error) {
	if err := r.seedState(ctx, spec); err != nil {
		return nil, err
	}
	return r.hub.Begin(ctx, roomwork.Room{Folder: r.instances.Workspace(spec.Session), Meta: r.instances.Merges(spec.Session)},
		dir.Workspace(), r.nameOf(spec))
}

// bring merges a passed turn into the room's folder, and tells the room of the files two agents changed. What may write to the
// room is checked again first, since a turn runs for minutes: a turn whose agent was taken out, or whose asker left, writes nothing.
func (r *roomRuns) bring(ctx context.Context, turn *roomwork.Turn, spec job.Spec, dir job.Dir) error {
	if why := r.mayNotWrite(spec, r.askedBy(dir)); why != "" {
		if _, err := r.instances.Notice(spec.Session, spec.Thread, why); err != nil {
			slog.Error("could not tell the room a turn wrote nothing", "room", spec.Session, "error", err)
		}
		decided(dir, evidence.Decision{Refused: why})
		return nil
	}
	outcome, err := turn.Merge(ctx)
	if err != nil {
		return err
	}
	decided(dir, decisionOf(outcome))
	if words := outcome.Words(); words != "" {
		if _, err := r.instances.Notice(spec.Session, spec.Thread, words); err != nil {
			slog.Error("could not tell the room about a conflict", "room", spec.Session, "error", err)
		}
	}
	return nil
}

// mayNotWrite is why a finished turn may not change the room's folder now, or nothing when it may.
func (r *roomRuns) mayNotWrite(spec job.Spec, asker string) string {
	held, found := r.instances.Get(spec.Session)
	if !found {
		return "the room is gone"
	}
	agent, in := held.AgentByID(spec.Agent)
	switch {
	case !in || agent.Pending:
		return r.nameOf(spec) + " was taken out of the room while it worked, so what it changed was not kept."
	case asker != "" && held.RoleOf(asker) == "":
		return asker + " left the room while " + agent.Name + " worked for them, so what it changed was not kept."
	}
	return ""
}

// nameOf is what the room calls the turn's agent.
func (r *roomRuns) nameOf(spec job.Spec) string {
	if held, found := r.instances.Get(spec.Session); found {
		if agent, found := held.AgentByID(spec.Agent); found {
			return agent.Name
		}
	}
	return spec.Agent
}

// seedState makes the agent's own state folder the first time it is needed, from what the room's shared state holds, so an
// agent that had a conversation before rooms ran at the same time carries on with it.
func (r *roomRuns) seedState(ctx context.Context, spec job.Spec) error {
	own := r.instances.AgentState(spec.Session, spec.Agent)
	if _, err := os.Stat(own); err == nil {
		return nil
	}
	next := own + ".seed"
	if err := os.RemoveAll(next); err != nil {
		return err
	}
	if err := os.MkdirAll(next, 0o755); err != nil {
		return err
	}
	if err := roomwork.CopyTree(ctx, r.instances.State(spec.Session), next, agentStateLimits); err != nil {
		return err
	}
	return os.Rename(next, filepath.Clean(own))
}

// decisionOf is what a merge did, as a turn's record keeps it.
func decisionOf(outcome roomwork.Outcome) evidence.Decision {
	decision := evidence.Decision{Kept: true, Changed: outcome.Changed}
	for _, each := range outcome.Joined {
		decision.Joined = append(decision.Joined, each.Path)
	}
	for _, each := range outcome.Conflicts {
		decision.Conflicts = append(decision.Conflicts, each.Path)
	}
	return decision
}

// decided puts what was done with a turn's changes in its signed record. A record that cannot take it is said, and the room's
// folder is as the decision left it either way.
func decided(dir job.Dir, decision evidence.Decision) {
	if err := evidence.Decide(dir, decision); err != nil {
		slog.Error("a turn's record does not say what was done with its changes", "job", filepath.Base(dir.Path()), "error", err)
	}
}
