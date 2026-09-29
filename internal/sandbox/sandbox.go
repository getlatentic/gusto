// Package sandbox is the box one job runs in.
package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/signing"
)

// Runtime is the container command. Podman takes the same arguments and runs no
// daemon at all, which matters on a small machine and to a customer who will not
// install one.
func Runtime() string {
	if runtime := os.Getenv("HAAS_CONTAINER"); runtime != "" {
		return runtime
	}
	return "docker"
}

// Image carrying the runner and the harness it will run. A customer builds
// their own and names it here.
func Image() string {
	if image := os.Getenv("HAAS_IMAGE"); image != "" {
		return image
	}
	return "haas:dev"
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
	absolute, err := filepath.Abs(dir.Path())
	if err != nil {
		return 0, err
	}
	alias, err := ready(absolute)
	if err != nil {
		return 0, err
	}
	network := []string{
		"-e", "HAAS_MODEL_URL=" + inContainer(model),
		// Where a harness would have put a credential. The real one stays in the
		// proxy, and this is worth nothing once the job ends.
		"-e", "HAAS_MODEL_TOKEN=" + token,
	}
	if alias {
		network = append(network, "--add-host=host.docker.internal:host-gateway")
	}
	args, err := argv(absolute, network, mounts)
	if err != nil {
		return 0, err
	}
	return start(dir, args)
}

// start runs the container, then signs the manifest it left. The manifest is
// written inside the box, where the agent runs, so the key never goes in: the
// host signs once the box is gone, with the key HAAS_SIGNING_KEY names here.
func start(dir job.Dir, args []string) (int, error) {
	command := exec.Command(Runtime(), args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil && !isExit(err) {
		return 0, err
	}
	if err := evidence.SealAsConfigured(dir); err != nil {
		return 0, err
	}
	return command.ProcessState.ExitCode(), nil
}

// argv is the container's command line: the job directory, what reaches the
// network, the mounts, and what one job may take of the machine.
func argv(absolute string, network []string, mounts []Mount) ([]string, error) {
	signer, err := signerName()
	if err != nil {
		return nil, err
	}
	args := []string{"run", "--rm",
		"-v", absolute + ":/job",
		"-e", "HAAS_SIGNER=" + signer,
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

// signerName is the name the box writes into the manifest: HAAS_SIGNER, or the
// id of the key the host will sign with, so a record the host signs never
// calls itself unsigned. Reading the key here also stops a run whose key is
// unreadable before the agent does any work.
func signerName() (string, error) {
	key, keyed, err := signing.Configured()
	if name := os.Getenv("HAAS_SIGNER"); name != "" || err != nil || !keyed {
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
		"--cpus=" + share("HAAS_JOB_CPUS", "1"),
		"--memory=" + share("HAAS_JOB_MEMORY", "3g"),
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
