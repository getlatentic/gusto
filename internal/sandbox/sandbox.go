// Package sandbox is the box one task runs in.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/task"
)

// Containers is how this machine starts the containers tasks and programs run in.
type Containers struct {
	// Runtime is the container command. Podman takes the same arguments and runs no daemon at all, which matters on a small
	// machine and to a customer who will not install one.
	Runtime string
	// Image carries the runner and the agents it runs; a customer builds their own and names it here. ImageSource is where it
	// is fetched from when this machine does not have it, or nothing.
	Image, ImageSource string
	// CPUs and Memory are what one task may take of the machine.
	CPUs, Memory string
	// ToolImage is the image programs run in, or nothing for Image. ToolNetwork "open" gives them the runtime's network.
	// ToolCPUs and ToolMemory are what one program may take.
	ToolImage, ToolNetwork, ToolCPUs, ToolMemory string
	// Signing is who stands behind the records of the tasks run here.
	Signing signing.Identity
}

// Run the task in a container and hand back its exit code. The task directory is
// the only thing mounted, and the container is destroyed with the run.
// Mount is something outside the task directory that a container needs: a
// conversation's working directory, or the memory an agent keeps of it.
type Mount struct {
	From string
	To   string
	// Kept is a person's own home, kept between their tasks: the runner gives the agent only what it put there for this task,
	// and leaves the rest as the agent left it, rather than walking every file in it twice a task.
	Kept bool
}

// Home is the agent's home in a task's container, and KeptHomeEnv tells the runner that what is there is a person's own.
const (
	Home        = "/root"
	KeptHomeEnv = "GUSTO_HOME_KEPT"
)

// Run takes the address the container should ask for a model at. It is given
// rather than read from the environment because it is per task: the proxy mints
// a token for this run, and that token is in the address.
//
// The container keeps the runtime's default network, so this is the local case:
// a model on the host and no credential to keep out. A task with a proxy runs
// Sealed instead.
//
// The container is stopped when ctx ends.
func (c Containers) Run(ctx context.Context, dir task.Dir, model, token string, owner Owner, mounts ...Mount) (int, error) {
	absolute, err := filepath.Abs(dir.Path())
	if err != nil {
		return 0, err
	}
	alias, err := c.ready(absolute)
	if err != nil {
		return 0, err
	}
	network := []string{
		"-e", "GUSTO_MODEL_URL=" + inContainer(model),
		// Where an agent would have put a credential. The real one stays in the
		// proxy, and this is worth nothing once the task ends.
		"-e", "GUSTO_MODEL_TOKEN=" + token,
	}
	if alias {
		network = append(network, "--add-host=host.docker.internal:host-gateway")
	}
	args, err := c.argv(absolute, network, owner, mounts)
	if err != nil {
		return 0, err
	}
	return c.start(ctx, dir, args)
}

// start runs the container, then signs the manifest it left. The manifest is
// written inside the box, where the agent runs, so the key never goes in: the
// host signs once the box is gone.
func (c Containers) start(ctx context.Context, dir task.Dir, args []string) (int, error) {
	command := exec.CommandContext(ctx, c.Runtime, args...)
	// A stop is an interrupt to the runtime's client, which hands it on to the container's first process, `gusto inside`. That
	// ends the agent and writes down what it did. Killing the client would leave the container running and spending.
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = stopGrace
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	err := command.Run()
	if ctx.Err() != nil {
		// The client may have been killed after its grace, and the container with it not at all: it is removed by name.
		_ = exec.Command(c.Runtime, "rm", "-f", containerName(dir)).Run()
		_ = record.SealIfKeyed(dir, c.Signing.Key)
		return -1, ctx.Err()
	}
	if err != nil && !isExit(err) {
		return 0, err
	}
	if err := record.SealIfKeyed(dir, c.Signing.Key); err != nil {
		return 0, err
	}
	return command.ProcessState.ExitCode(), nil
}

// stopGrace is how long a container is given to write down what a stopped task did before its client is killed.
const stopGrace = 60 * time.Second

// containerName is the name a task's container has, so that it can be removed by it whatever became of the client that
// started it. A task's id is already a name a container may have.
func containerName(dir task.Dir) string {
	return "gusto-" + filepath.Base(filepath.Clean(dir.Path()))
}

// argv is the container's command line: the task directory, whose it is, what reaches the
// network, the mounts, and what one task may take of the machine.
// The box is told the name the host will sign as, so a record the host signs never calls itself unsigned.
func (c Containers) argv(absolute string, network []string, owner Owner, mounts []Mount) ([]string, error) {
	args := []string{"run", "--rm", "--name", "gusto-" + filepath.Base(absolute),
		"-v", absolute + ":/job",
		"-e", signing.NameEnv + "=" + c.Signing.Says(),
	}
	args = append(args, owner.labels(filepath.Base(absolute))...)
	args = append(args, network...)
	for _, mount := range mounts {
		absolute, err := filepath.Abs(mount.From)
		if err != nil {
			return nil, err
		}
		args = append(args, "-v", absolute+":"+mount.To)
		if mount.Kept {
			args = append(args, "-e", KeptHomeEnv+"=1")
		}
	}
	args = append(args, c.Limits()...)
	args = append(args, Hardening()...)
	args = append(args, c.Image, "inside", "/job")
	return args, nil
}

// inContainer rewrites a model address the host reaches on loopback into one a
// container reaches.
func inContainer(url string) string {
	for _, local := range []string{"localhost", "127.0.0.1"} {
		url = strings.Replace(url, local, "host.docker.internal", 1)
	}
	return url
}

// ready checks the container can read the task directory, and reports whether the
// runtime needs the host-gateway alias to resolve host.docker.internal.
//
// Both answers come from one container start because both failures are silent
// otherwise. A bind mount of a path the runtime does not share hands over an
// empty directory rather than failing. And Docker Desktop already resolves
// host.docker.internal to the host, so adding the alias there replaces a working
// address with the virtual machine's own bridge gateway, which reaches the
// virtual machine instead of the host.
func (c Containers) ready(absolute string) (bool, error) {
	if err := c.EnsureImage(context.Background()); err != nil {
		return false, err
	}
	check := exec.Command(c.Runtime, "run", "--rm", "-v", absolute+":/job",
		"--entrypoint", "sh", c.Image, "-c",
		"test -f /job/job.json || exit 3; getent hosts host.docker.internal >/dev/null || exit 4")
	err := check.Run()
	switch code := check.ProcessState.ExitCode(); {
	case err == nil:
		return false, nil
	case code == 4:
		return true, nil
	case code == 3:
		return false, fmt.Errorf("the container cannot read %s/job.json: check that the path is shared with the container runtime", absolute)
	default:
		return false, fmt.Errorf("could not start a container from %s: %w", c.Image, err)
	}
}

// Limits are the flags that bound what one task takes of the machine. A runner
// often sits beside something else the owner cares about, and a limit larger
// than the machine is not a limit: the task takes what it likes and the kernel
// kills whatever it reaches first. They are exported so a check before a task
// can start a container under the same limits the task will run under, because
// a runtime can refuse a limit outright.
func (c Containers) Limits() []string {
	return []string{"--cpus=" + c.CPUs, "--memory=" + c.Memory, "--pids-limit=512"}
}

// Hardening is what a task's container is denied. The runner is root in the box
// so that it alone can write the record; it needs to give the agent its files
// (CHOWN), read them whatever their mode (DAC_OVERRIDE), and start the agent
// as the agent (SETUID, SETGID). Nothing else, so a process that gets root in the
// box has no more than that, and no program can gain a privilege by being
// setuid, which is what no-new-privileges refuses.
func Hardening() []string {
	return []string{
		"--cap-drop=ALL",
		"--cap-add=CHOWN", "--cap-add=DAC_OVERRIDE", "--cap-add=FOWNER", "--cap-add=SETUID", "--cap-add=SETGID",
		"--security-opt=no-new-privileges",
	}
}

func isExit(err error) bool {
	_, ok := err.(*exec.ExitError)
	return ok
}
