// Package container runs each turn's agent in a container of the runner image, which is the product's own box.
package container

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// Config is what a server's containers are made with.
type Config struct {
	// Agents are the agents a container may be asked to run.
	Agents []string
	// Org is the organization whose server this is, which labels each container.
	Org string
	// Homes is where an organization's server keeps each person's home beside their computer's record, or nothing.
	Homes string
	// Open leaves a container on the runtime's network, where it reaches the proxy on a port. Otherwise it has no network
	// and reaches the proxy through a socket.
	Open bool
	// Containers is how containers are started, and who signs their records.
	Containers sandbox.Containers
}

// Backend runs turns in containers.
type Backend struct {
	cfg     Config
	once    sync.Once
	offered atomic.Pointer[[]catalog.Entry]
}

// New is a backend of containers made with cfg.
func New(cfg Config) *Backend { return &Backend{cfg: cfg} }

func (b *Backend) String() string {
	return "in a container, " + b.cfg.Containers.Runtime + " " + b.cfg.Containers.Image
}

// Check finds nothing to refuse: a computer with no runtime or no image is said agent by agent in Offered.
func (b *Backend) Check() error { return nil }

// Reach is a socket for a container with no network, and the runtime's network otherwise.
func (b *Backend) Reach() backend.Reach {
	if b.cfg.Open {
		return backend.Network
	}
	return backend.Socket
}

// Offered is what the runner image carries, asked once of a container because each ask starts one, and again when an image
// that was missing has been fetched.
func (b *Backend) Offered() []catalog.Entry {
	b.once.Do(func() {
		first := b.survey()
		b.offered.Store(&first)
	})
	return *b.offered.Load()
}

func (b *Backend) survey() []catalog.Entry {
	return catalog.Survey(b.cfg.Containers.Runtime, b.cfg.Containers.Image, b.cfg.Agents)
}

// Start fetches the runner image in the background when its containers can run nothing yet and it can be fetched.
func (b *Backend) Start(ctx context.Context) (func(), error) {
	offered := b.Offered()
	if b.cfg.Containers.ImageSource == "" {
		return func() {}, nil
	}
	waiting, stop := context.WithCancel(ctx)
	done := awaitImage(waiting, offered, b.cfg.Containers.EnsureImage, b.survey, func(fresh []catalog.Entry) { b.offered.Store(&fresh) }, imageRetry)
	return func() { stop(); <-done }, nil
}

// Run runs the turn's agent in a container. With a proxy the container is given a token that stands in for the credential;
// without one it asks the model directly.
func (b *Backend) Run(ctx context.Context, turn backend.Turn) (int, error) {
	mounts, err := b.mounts(turn)
	if err != nil {
		return 0, err
	}
	owner, exit := b.owner(turn), turn.Exit
	if exit.Door == nil {
		return b.cfg.Containers.Run(ctx, turn.Dir, exit.Direct, "", owner, mounts...)
	}
	token, err := exit.Door.Token(turn.Dir, turn.Spec)
	if err != nil {
		return 0, err
	}
	if !b.cfg.Open {
		return b.cfg.Containers.Sealed(ctx, turn.Dir, exit.Socket, token, owner, mounts...)
	}
	return b.cfg.Containers.Run(ctx, turn.Dir, "http://localhost:"+exit.Port+"/model/v1", token, owner, mounts...)
}

// owner is whose a turn's container is: the organization, the person who pays as the same opaque id their computer has, and
// the agent's instance.
func (b *Backend) owner(turn backend.Turn) sandbox.Owner {
	owner := sandbox.Owner{Org: b.cfg.Org, Instance: turn.Spec.Session}
	if turn.Person != "" {
		owner.Person = computer.ID(b.cfg.Org, turn.Person)
	}
	return owner
}

// mounts are what a turn needs that outlives its container: the person's home, and the conversation's workspace and memory.
func (b *Backend) mounts(turn backend.Turn) ([]sandbox.Mount, error) {
	home, err := b.keptHome(turn)
	if err != nil {
		return nil, err
	}
	return append(home, staying(turn)...), nil
}

// keptHome is the home of the person a turn counts against, mounted where the agent's is: what they install and keep there is
// there at their next turn, as it is in a computer of their own. A task nobody asked for has none, and nor does a room's turn,
// which sees only the room.
func (b *Backend) keptHome(turn backend.Turn) ([]sandbox.Mount, error) {
	if b.cfg.Homes == "" || turn.Person == "" || turn.InRoom() {
		return nil, nil
	}
	home := filepath.Join(b.cfg.Homes, computer.ID(b.cfg.Org, turn.Person), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	return []sandbox.Mount{{From: home, To: sandbox.Home, Kept: true}}, nil
}

// staying is the conversation's working directory and the memory the agent keeps of it. An agent stores that memory in its
// own data directory rather than beside the files it edits, which is why there are two. A room's agent works on a copy of the
// room's folder that is in its task directory already, so only the memory is mounted.
func staying(turn backend.Turn) []sandbox.Mount {
	if turn.Spec.Session == "" {
		return nil
	}
	memory := sandbox.Mount{From: turn.State, To: "/root/.local/share"}
	if turn.Workspace == "" {
		return []sandbox.Mount{memory}
	}
	return []sandbox.Mount{{From: turn.Workspace, To: "/job/workspace"}, memory}
}
