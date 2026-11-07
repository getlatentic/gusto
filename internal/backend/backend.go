// Package backend names what gusto's server needs of a sandbox: a box that runs one turn's agent and is stopped when the
// turn's context ends. Each kind of box is a package beside this one: container, vm and process.
package backend

import (
	"context"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/pkg/task"
)

// Backend runs turns in boxes of one kind. One Backend serves every turn of a server, and runs turns at the same time.
type Backend interface {
	// String says where a turn runs, as a benchmark's table says it: "in a container, docker gusto-runner".
	String() string
	// Check says why this computer cannot make the backend's boxes, or nil when it can.
	Check() error
	// Reach is how a box reaches the proxy, which listens to suit.
	Reach() Reach
	// Start begins what the backend keeps up while the server runs, such as fetching its image. stop ends it and waits.
	Start(ctx context.Context) (stop func(), err error)
	// Offered is which agents the backend's boxes can run, as last found. It is cheap to ask: a backend whose finding
	// starts a box keeps what it found.
	Offered() []catalog.Entry
	// Run runs a turn's agent in a box and returns its exit code. The box is stopped when ctx ends.
	Run(ctx context.Context, turn Turn) (int, error)
}

// Reach is how a box reaches the proxy.
type Reach int

const (
	// Socket is a box with no network, which reaches the proxy through a socket on this computer.
	Socket Reach = iota
	// Network is a box that reaches the proxy over a network the runtime makes, so the proxy listens on every interface.
	Network
	// Loopback is a box that reaches this computer's loopback, so the proxy listens there and nowhere else.
	Loopback
)

// Turn is one run of an agent, with what the server knows of it that the task's own folder does not say.
type Turn struct {
	Dir  task.Dir
	Spec task.Spec
	// Person is who the turn counts against, as an address, or nothing for a token or a script.
	Person string
	// AgentFolder is the instance's folder a conversation's turn works on, or nothing for a task of its own and a room's turn,
	// whose copy of the room's folder is in Dir already.
	AgentFolder string
	// State is where the agent keeps its memory of the conversation, or nothing for a task of its own.
	State string
	// Exit is how the turn's agent reaches its model.
	Exit Exit
}

// InRoom says whether the turn is one of a room's agents. Only a room's turns name an agent.
func (t Turn) InRoom() bool { return t.Spec.Session != "" && t.Spec.Agent != "" }

// Exit is how a box's agent reaches its model.
type Exit struct {
	// Door is the proxy, which holds the credential. It is nil when this deployment holds none, and a box asks Direct.
	Door Door
	// Direct is the model's own address, for a deployment with no proxy.
	Direct string
	// Socket is where a box with no network reaches the proxy.
	Socket string
	// Port is the proxy's port, for a box that reaches it over a network or the loopback.
	Port string
}

// Door is the proxy as a box's backend asks it for the turn's way out.
type Door interface {
	// Token is what a box is given in place of a credential, or nothing when its agent needs neither a model nor a tool
	// server. A turn that may not reach its model is refused here, in its record.
	Token(dir task.Dir, spec task.Spec) (string, error)
	// SandboxToken is Token for a box that holds a key of its own: a secret its forwarder trades for a run token bound to
	// that key, or a plain token when the organization's admins chose them. bound says which.
	SandboxToken(dir task.Dir, spec task.Spec) (token string, bound bool, err error)
	// RecordRun keeps, in the run's record, who and what it is for, the sandbox it runs in and how it got its token.
	RecordRun(dir task.Dir, spec task.Spec, sandbox string, bound bool) error
}
