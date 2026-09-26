// Package sandbox is the box one job runs in.
package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
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
func Run(dir job.Dir) (int, error) {
	absolute, err := filepath.Abs(dir.Path())
	if err != nil {
		return 0, err
	}
	alias, err := ready(absolute)
	if err != nil {
		return 0, err
	}
	args := []string{"run", "--rm",
		"-v", absolute + ":/job",
		"-e", "HAAS_MODEL_URL=" + inContainer(harness.ModelURL()),
		"-e", "HAAS_SIGNER=" + os.Getenv("HAAS_SIGNER"),
		"--cpus=" + share("HAAS_JOB_CPUS", "1"),
		"--memory=" + share("HAAS_JOB_MEMORY", "3g"),
		"--pids-limit=512"}
	if alias {
		args = append(args, "--add-host=host.docker.internal:host-gateway")
	}
	args = append(args, Image(), "inside", "/job")
	command := exec.Command(Runtime(), args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil && !isExit(err) {
		return 0, err
	}
	return command.ProcessState.ExitCode(), nil
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
