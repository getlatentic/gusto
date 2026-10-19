// Package sandbox is the box one job runs in.
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/signing"
)

// Runtime is the container command. Podman takes the same arguments and runs no
// daemon at all, which matters on a small machine and to a customer who will not
// install one.
func Runtime() string {
	if runtime := os.Getenv("GUSTO_CONTAINER"); runtime != "" {
		return runtime
	}
	return "docker"
}

// Image carrying the runner and the harness it will run. A customer builds
// their own and names it here.
func Image() string {
	if image := os.Getenv("GUSTO_IMAGE"); image != "" {
		return image
	}
	return "gusto:dev"
}

// Run the job in a container and hand back its exit code. The job directory is
// the only thing mounted, and the container is destroyed with the run.
// Mount is something outside the job directory that a container needs: a
// conversation's working directory, or the memory a harness keeps of it.
type Mount struct {
	From string
	To   string
}

// Run takes the address the container should ask for a model at. It is given
// rather than read from the environment because it is per job: the proxy mints
// a token for this run, and that token is in the address.
//
// The container keeps the runtime's default network, so this is the local case:
// a model on the host and no credential to keep out. A job with a proxy runs
// Sealed instead.
func Run(dir job.Dir, model, token string, mounts ...Mount) (int, error) {
	return RunContext(context.Background(), dir, model, token, mounts...)
}

// RunContext is Run that stops the container when its context ends.
func RunContext(ctx context.Context, dir job.Dir, model, token string, mounts ...Mount) (int, error) {
	absolute, err := filepath.Abs(dir.Path())
	if err != nil {
		return 0, err
	}
	alias, err := ready(absolute)
	if err != nil {
		return 0, err
	}
	network := []string{
		"-e", "GUSTO_MODEL_URL=" + inContainer(model),
		// Where a harness would have put a credential. The real one stays in the
		// proxy, and this is worth nothing once the job ends.
		"-e", "GUSTO_MODEL_TOKEN=" + token,
	}
	if alias {
		network = append(network, "--add-host=host.docker.internal:host-gateway")
	}
	args, err := argv(absolute, network, mounts)
	if err != nil {
		return 0, err
	}
	return start(ctx, dir, args)
}

// start runs the container, then signs the manifest it left. The manifest is
// written inside the box, where the agent runs, so the key never goes in: the
// host signs once the box is gone, with the key GUSTO_SIGNING_KEY names here.
func start(ctx context.Context, dir job.Dir, args []string) (int, error) {
	command := exec.CommandContext(ctx, Runtime(), args...)
	// A stop is an interrupt to the runtime's client, which hands it on to the container's first process, `gusto inside`. That
	// ends the harness and writes down what it did. Killing the client would leave the container running and spending.
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = stopGrace
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	err := command.Run()
	if ctx.Err() != nil {
		// The client may have been killed after its grace, and the container with it not at all: it is removed by name.
		_ = exec.Command(Runtime(), "rm", "-f", containerName(dir)).Run()
		_ = evidence.SealAsConfigured(dir)
		return -1, ctx.Err()
	}
	if err != nil && !isExit(err) {
		return 0, err
	}
	if err := evidence.SealAsConfigured(dir); err != nil {
		return 0, err
	}
	return command.ProcessState.ExitCode(), nil
}

// stopGrace is how long a container is given to write down what a stopped job did before its client is killed.
const stopGrace = 60 * time.Second

// containerName is the name a job's container has, so that it can be removed by it whatever became of the client that
// started it. A job's id is already a name a container may have.
func containerName(dir job.Dir) string {
	return "gusto-" + filepath.Base(filepath.Clean(dir.Path()))
}

// argv is the container's command line: the job directory, what reaches the
// network, the mounts, and what one job may take of the machine.
func argv(absolute string, network []string, mounts []Mount) ([]string, error) {
	signer, err := signerName()
	if err != nil {
		return nil, err
	}
	args := []string{"run", "--rm", "--name", "gusto-" + filepath.Base(absolute),
		"-v", absolute + ":/job",
		"-e", "GUSTO_SIGNER=" + signer,
	}
	args = append(args, network...)
	for _, mount := range mounts {
		absolute, err := filepath.Abs(mount.From)
		if err != nil {
			return nil, err
		}
		args = append(args, "-v", absolute+":"+mount.To)
	}
	args = append(args, Limits()...)
	args = append(args, Hardening()...)
	args = append(args, Image(), "inside", "/job")
	return args, nil
}

// signerName is the name the box writes into the manifest: GUSTO_SIGNER, or the
// id of the key the host will sign with, so a record the host signs never
// calls itself unsigned. Reading the key here also stops a run whose key is
// unreadable before the agent does any work.
func signerName() (string, error) {
	key, keyed, err := signing.Configured()
	if name := os.Getenv("GUSTO_SIGNER"); name != "" || err != nil || !keyed {
		return name, err
	}
	return key.ID(), nil
}

// inContainer rewrites a model address the host reaches on loopback into one a
// container reaches.
func inContainer(url string) string {
	for _, local := range []string{"localhost", "127.0.0.1"} {
		url = strings.Replace(url, local, "host.docker.internal", 1)
	}
	return url
}

// ready checks the container can read the job directory, and reports whether the
// runtime needs the host-gateway alias to resolve host.docker.internal.
//
// Both answers come from one container start because both failures are silent
// otherwise. A bind mount of a path the runtime does not share hands over an
// empty directory rather than failing. And Docker Desktop already resolves
// host.docker.internal to the host, so adding the alias there replaces a working
// address with the virtual machine's own bridge gateway, which reaches the
// virtual machine instead of the host.
func ready(absolute string) (bool, error) {
	check := exec.Command(Runtime(), "run", "--rm", "-v", absolute+":/job",
		"--entrypoint", "sh", Image(), "-c",
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
		return false, fmt.Errorf("could not start a container from %s: %w", Image(), err)
	}
}

// Limits are the flags that bound what one job takes of the machine. They are
// exported so a check before a job can start a container under the same limits
// the job will run under, because a runtime can refuse a limit outright.
func Limits() []string {
	return []string{
		"--cpus=" + share("GUSTO_JOB_CPUS", "1"),
		"--memory=" + share("GUSTO_JOB_MEMORY", "3g"),
		"--pids-limit=512",
	}
}

// Hardening is what a job's container is denied. The runner is root in the box
// so that it alone can write the record; it needs to give the agent its files
// (CHOWN), read them whatever their mode (DAC_OVERRIDE), and start the harness
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

// share is what one job may take of the machine. A runner often sits beside
// something else the owner cares about, and a limit larger than the machine is
// not a limit: the job takes what it likes and the kernel kills whatever it
// reaches first.
func share(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
