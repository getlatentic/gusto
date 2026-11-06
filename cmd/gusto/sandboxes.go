package main

import (
	"context"
	"os"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/backend/container"
	"github.com/getlatentic/gusto/internal/backend/process"
	"github.com/getlatentic/gusto/internal/backend/vm"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// openBackend is the sandbox turns run in, as GUSTO_SANDBOX names it: here and process run a turn in this process, openshell in
// a virtual machine on a gateway, and anything else, container among it, in a container. It is the one place a backend is made.
// shared is where the apps agents built are kept, and proxyPort the proxy's port; a benchmark passes neither.
func openBackend(shared *apps.Store, proxyPort string) (backend.Backend, error) {
	agents, org, open := harness.Names(), os.Getenv("GUSTO_ORG"), os.Getenv("GUSTO_JOB_NETWORK") == "open"
	switch name := os.Getenv("GUSTO_SANDBOX"); name {
	case "here", "process":
		return process.New(process.Config{Agents: agents, Walled: name == "process", Open: open}), nil
	case "openshell":
		gateway, err := openshell.FromEnv()
		if err != nil {
			return nil, err
		}
		computers, err := computersFromEnv()
		if err != nil {
			return nil, err
		}
		return vm.New(vm.Config{
			Gateway: gateway, Agents: agents, Computers: computers, Org: org, Cell: setting("GUSTO_CELL", "local"), Apps: shared,
			Spare: os.Getenv("GUSTO_SPARE_COMPUTER") == "on", ProxyPort: proxyPort,
			Idle: time.Duration(number("GUSTO_COMPUTER_IDLE_MINUTES", 30)) * time.Minute,
		}), nil
	}
	return container.New(container.Config{
		Agents: agents, Org: org, Homes: os.Getenv("GUSTO_COMPUTERS"), Open: open, Fetched: os.Getenv(sandbox.ImageSource) != "",
	}), nil
}

// runsHere says turns run in this process, so the agents are the programs on this computer, and a tool server, which is a
// program in a container, has no container to run in.
func runsHere() bool {
	name := os.Getenv("GUSTO_SANDBOX")
	return name == "here" || name == "process"
}

// turns runs each task as a turn in the backend's boxes. The checkout happens here, outside any box, so a repository credential
// never goes in with the agent. A turn learns what the agent calls the conversation, and the next turn is handed it back;
// without that every turn starts again.
func turns(b backend.Backend, way *WayOut, instances *instance.Store, index *jobs.Index, rooms *roomRuns) jobs.StoppableWork {
	return func(ctx context.Context, dir job.Dir) (int, error) {
		spec, err := job.Read(dir)
		if err != nil {
			return 0, err
		}
		if err := runner.Fill(spec, dir); err != nil {
			return 0, err
		}
		defer way.Done(dir)
		turn := turnOf(instances, index, dir, spec)
		turn.Exit = way.exit()
		exit, err := rooms.around(ctx, spec, dir, func() (int, error) { return b.Run(ctx, turn) })
		if manifest, read := evidence.Read(dir); read == nil {
			learnConversation(instances, spec, manifest.Session)
		}
		return exit, err
	}
}

// turnOf is what the server knows of a task that its folder does not say: who pays for it, and for a turn in a conversation,
// the instance's workspace and the agent's memory. A room's turn works on a copy of the room's folder that roomRuns puts in the
// task's, so it is given no workspace.
func turnOf(instances *instance.Store, index *jobs.Index, dir job.Dir, spec job.Spec) backend.Turn {
	turn := backend.Turn{Dir: dir, Spec: spec, Person: personWhoPays(index, dir)}
	if spec.Session == "" {
		return turn
	}
	turn.State = stateDir(instances, spec)
	if !isRoomTurn(spec) {
		turn.Workspace = instances.Workspace(spec.Session)
	}
	return turn
}
