package main

import (
	"context"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/backend/container"
	"github.com/getlatentic/gusto/internal/backend/process"
	"github.com/getlatentic/gusto/internal/backend/vm"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/roomwork"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/settings"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/task"
)

// sandboxSettings is what turns run in and how many at once: the kind of box, its network, and for an organization's server
// the computers its people have.
type sandboxSettings struct {
	sandbox, taskNetwork, computers, node           string
	spare                                           bool
	idleMinutes, concurrency, roomCopyMB, roomFiles int
	containers                                      containerSettings
	openShell                                       openShellSettings
}

func (b *sandboxSettings) declare(set *settings.Set) {
	set.Choice(&b.sandbox, "GUSTO_SANDBOX", "container", []string{"container", "openshell", "process", "here"}, "how tasks are held")
	set.Choice(&b.taskNetwork, "GUSTO_TASK_NETWORK", "", []string{"open"}, "open gives containers the runtime's network and the proxy on GUSTO_PROXY_ADDR")
	set.Renamed("GUSTO_TASK_NETWORK", "GUSTO_JOB_NETWORK")
	set.Int(&b.concurrency, "GUSTO_TASK_CONCURRENCY", 1, 1, "how many tasks run at a time")
	set.Renamed("GUSTO_TASK_CONCURRENCY", "GUSTO_JOB_CONCURRENCY")
	b.containers.declare(set)
	b.openShell.declare(set)
	set.String(&b.computers, "GUSTO_COMPUTERS", "", "the directory of the records of the computers an organization's people have")
	set.String(&b.node, "GUSTO_NODE", "local", "the node new computers are made on")
	set.Renamed("GUSTO_NODE", "GUSTO_CELL")
	set.Bool(&b.spare, "GUSTO_SPARE_COMPUTER", "keep one blank computer ready for the next person who has none")
	set.Int(&b.idleMinutes, "GUSTO_COMPUTER_IDLE_MINUTES", 30, 1, "how long a computer nobody uses runs before it is stopped, in minutes")
	set.Int(&b.roomCopyMB, "GUSTO_ROOM_COPY_MB", 4096, 1, "the most a room's folder may hold for an agent's turn to copy it, in megabytes")
	set.Int(&b.roomFiles, "GUSTO_ROOM_COPY_FILES", 200000, 1, "the most files a room's folder may hold for an agent's turn to copy it")
}

// roomCopy is how much of a room's folder an agent's turn may copy.
func (b sandboxSettings) roomCopy() roomwork.Limits {
	return roomwork.Limits{Bytes: int64(b.roomCopyMB) << 20, Files: b.roomFiles}
}

// isolation is what was set about the box tasks are held in. The kind is what was decided, which is nothing when the strongest
// is to be chosen.
func (b sandboxSettings) isolation(decided string) isolation.Settings {
	return isolation.Settings{
		Sandbox: decided, Container: b.containers.c.Runtime, Image: b.containers.c.Image, TaskNetwork: b.taskNetwork,
		OpenShell: b.openShell.cfg.Binary, OpenShellImage: b.openShell.cfg.Image, OpenShellGateway: b.openShell.cfg.Gateway,
	}
}

// here says turns run in this process, so the agents are the programs on this computer, and a tool server, which is a program
// in a container, has no container to run in.
func (b sandboxSettings) here() bool { return b.sandbox == "here" || b.sandbox == "process" }

// openBackend is the sandbox turns run in, as GUSTO_SANDBOX names it: here and process run a turn in this process, openshell in
// a virtual machine on a gateway, and container in a container. It is the one place a backend is made. signs stands behind each
// turn's record, shared is where the apps agents built are kept, and proxyPort is the proxy's port; a benchmark passes no apps and
// no port.
func openBackend(cfg serveSettings, signs signing.Identity, shared *apps.Store, proxyPort string) (backend.Backend, error) {
	boxes, org := cfg.boxes, cfg.core.org
	agents, open := harness.Names(), boxes.taskNetwork == "open"
	switch boxes.sandbox {
	case "here", "process":
		return process.New(process.Config{Agents: agents, Walled: boxes.sandbox == "process", Open: open, Signing: signs}), nil
	case "openshell":
		computers, err := computersOf(boxes.computers, org, cfg.served.quota)
		if err != nil {
			return nil, err
		}
		return vm.New(vm.Config{
			Gateway: boxes.openShell.gateway(cfg.way.allowedHosts), Agents: agents, Computers: computers, Org: org, Node: boxes.node,
			Apps: shared, Spare: boxes.spare, ProxyPort: proxyPort, Idle: time.Duration(boxes.idleMinutes) * time.Minute, Signing: signs,
		}), nil
	}
	return container.New(container.Config{Agents: agents, Org: org, Homes: boxes.computers, Open: open, Containers: boxes.containers.signedBy(signs)}), nil
}

// turns runs each task as a turn in the backend's boxes. The checkout happens here, outside any box, so a repository credential
// never goes in with the agent. A turn learns what the agent calls the conversation, and the next turn is handed it back;
// without that every turn starts again.
func turns(b backend.Backend, way *WayOut, instances *instance.Store, index *tasks.Index, rooms *roomRuns) tasks.StoppableWork {
	return func(ctx context.Context, dir task.Dir) (int, error) {
		spec, err := task.Read(dir)
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
		if manifest, read := record.Read(dir); read == nil {
			learnConversation(instances, spec, manifest.Session)
		}
		return exit, err
	}
}

// turnOf is what the server knows of a task that its folder does not say: who pays for it, and for a turn in a conversation,
// the instance's folder and the agent's memory. A room's turn works on a copy of the room's folder that roomRuns puts in the
// task's, so it is given no folder.
func turnOf(instances *instance.Store, index *tasks.Index, dir task.Dir, spec task.Spec) backend.Turn {
	turn := backend.Turn{Dir: dir, Spec: spec, Person: personWhoPays(index, dir)}
	if spec.Session == "" {
		return turn
	}
	turn.State = stateDir(instances, spec)
	if !isRoomTurn(spec) {
		turn.AgentFolder = instances.AgentFolder(spec.Session)
	}
	return turn
}
