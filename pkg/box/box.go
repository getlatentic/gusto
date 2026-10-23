// Package box is the contract between gusto's runner and a place an agent runs that the
// runner only drives: a sandbox on a gateway, or a machine somebody else lends. The
// runner stays outside, writes the signed record itself and asks the box for the rest.
package box

import (
	"context"
	"os/exec"

	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// Box is a place an agent runs that this process only drives, such as a
// sandbox on a gateway. The runner stays outside and writes the record itself,
// so what runs in the box has nothing of the record to write, and a sandbox
// cannot be made to run as root for the sake of one.
type Box interface {
	// Start makes the box and puts the workspace in it.
	Start(ctx context.Context, workspace string) error
	// Wrap is a command that runs the one given inside the box, with its
	// standard input, output and exit status carried through.
	Wrap(ctx context.Context, command *exec.Cmd) *exec.Cmd
	// Attach is Wrap for a command that is spoken to while it runs: what is
	// written to its standard input arrives at once, and what it writes is read
	// as it is written.
	Attach(ctx context.Context, command *exec.Cmd) (*exec.Cmd, error)
	// Fetch replaces into with the workspace as the box now holds it.
	Fetch(into string) error
	// Equip puts the task's skills where the agent finds them in the box.
	Equip(ctx context.Context, dir job.Dir, spec job.Spec, adapter harness.Adapter) error
	// HostModelURL is where the proxy is from this process, which is not where the
	// box sees it: the box names this machine by a name of its own.
	HostModelURL() string
	// Env is what an agent in the box is started with: where its model is, and
	// nothing of this process's environment.
	Env() harness.Env
	// Workdir is where the workspace is inside the box.
	Workdir() string
	// Guard is what the box's own guard recorded, and the policy it enforced, to
	// be kept in the record.
	Guard() (events, policy []byte, err error)
	// Close removes the box.
	Close() error
}

// Files the box's guard leaves in the record. They are written before the
// manifest hashes out/, so they are signed with the rest.
const (
	GuardEvents = "sandbox-events.log"
	GuardPolicy = "sandbox-policy.json"
)
