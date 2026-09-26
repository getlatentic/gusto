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
	if err := visible(absolute); err != nil {
		return 0, err
	}
	model := strings.Replace(harness.ModelURL(), "localhost", "host.docker.internal", 1)
	command := exec.Command("docker", "run", "--rm",
		"-v", absolute+":/job",
		"-e", "HAAS_MODEL_URL="+model,
		"-e", "HAAS_SIGNER="+os.Getenv("HAAS_SIGNER"),
		"--cpus=1", "--memory=3g", "--pids-limit=512",
		"--add-host=host.docker.internal:host-gateway",
		Image(), "inside", "/job")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if !asExit(err, &exit) {
			return 0, err
		}
	}
	return command.ProcessState.ExitCode(), nil
}

func asExit(err error, into **exec.ExitError) bool {
	exit, ok := err.(*exec.ExitError)
	if ok {
		*into = exit
	}
	return ok
}

// visible checks the container can read the job directory before any work
// starts. A bind mount of a path the container runtime does not share hands the
// container a different, empty directory without saying so, and the run then
// fails somewhere that does not name the cause.
func visible(absolute string) error {
	check := exec.Command("docker", "run", "--rm",
		"-v", absolute+":/job", "--entrypoint", "test", Image(), "-f", "/job/job.json")
	if err := check.Run(); err != nil {
		return fmt.Errorf("the container cannot read %s/job.json: check that the path is shared with the container runtime", absolute)
	}
	return nil
}
