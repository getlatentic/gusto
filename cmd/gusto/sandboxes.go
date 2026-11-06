package main

import (
	"context"
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
	"github.com/getlatentic/gusto/internal/roomwork"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/internal/settings"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// sandboxSettings is what turns run in and how many at once: the kind of box, its network, and for an organization's server
// the computers its people have.
type sandboxSettings struct {
	sandbox, jobNetwork, computers, cell, imageSource string
	spare                                             bool
	idleMinutes, concurrency, roomCopyMB, roomFiles   int
}

func (b *sandboxSettings) declare(set *settings.Set) {
	set.Choice(&b.sandbox, "GUSTO_SANDBOX", "container", []string{"container", "openshell", "process", "here"}, "how tasks are held")
	set.Choice(&b.jobNetwork, "GUSTO_JOB_NETWORK", "", []string{"open"}, "open gives containers the runtime's network and the proxy on GUSTO_PROXY_ADDR")
	set.Int(&b.concurrency, "GUSTO_JOB_CONCURRENCY", 1, 1, "how many tasks run at a time")
	set.String(&b.imageSource, sandbox.ImageSource, "", "where the runner image is fetched from when this computer does not have it")
	set.String(&b.computers, "GUSTO_COMPUTERS", "", "the directory of the records of the computers an organization's people have")
	set.String(&b.cell, "GUSTO_CELL", "local", "where a new computer is made")
	set.Bool(&b.spare, "GUSTO_SPARE_COMPUTER", "keep one blank computer ready for the next person who has none")
	set.Int(&b.idleMinutes, "GUSTO_COMPUTER_IDLE_MINUTES", 30, 1, "how long a computer nobody uses runs before it is stopped, in minutes")
	set.Int(&b.roomCopyMB, "GUSTO_ROOM_COPY_MB", 4096, 1, "the most a room's folder may hold for an agent's turn to copy it, in megabytes")
	set.Int(&b.roomFiles, "GUSTO_ROOM_COPY_FILES", 200000, 1, "the most files a room's folder may hold for an agent's turn to copy it")
}

// roomCopy is how much of a room's folder an agent's turn may copy.
func (b sandboxSettings) roomCopy() roomwork.Limits {
	return roomwork.Limits{Bytes: int64(b.roomCopyMB) << 20, Files: b.roomFiles}
}

// here says turns run in this process, so the agents are the programs on this computer, and a tool server, which is a program
// in a container, has no container to run in.
func (b sandboxSettings) here() bool { return b.sandbox == "here" || b.sandbox == "process" }

// openBackend is the sandbox turns run in, as GUSTO_SANDBOX names it: here and process run a turn in this process, openshell in
// a virtual machine on a gateway, and container in a container. It is the one place a backend is made. org is the organization
// the server is, quota the file of what it may run, shared where the apps agents built are kept, and proxyPort the proxy's port;
// a benchmark passes no apps and no port.
func openBackend(cfg sandboxSettings, org, quota string, shared *apps.Store, proxyPort string) (backend.Backend, error) {
	agents, open := harness.Names(), cfg.jobNetwork == "open"
	switch cfg.sandbox {
	case "here", "process":
		return process.New(process.Config{Agents: agents, Walled: cfg.sandbox == "process", Open: open}), nil
	case "openshell":
		gateway, err := openshell.FromEnv()
		if err != nil {
			return nil, err
		}
		computers, err := computersOf(cfg.computers, org, quota)
		if err != nil {
			return nil, err
		}
		return vm.New(vm.Config{
			Gateway: gateway, Agents: agents, Computers: computers, Org: org, Cell: cfg.cell, Apps: shared,
			Spare: cfg.spare, ProxyPort: proxyPort, Idle: time.Duration(cfg.idleMinutes) * time.Minute,
		}), nil
	}
	return container.New(container.Config{Agents: agents, Org: org, Homes: cfg.computers, Open: open, Fetched: cfg.imageSource != ""}), nil
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
