package sandbox

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// A directory the container cannot read the job from has to be named as the
// problem. Docker hands over an empty directory rather than refusing a mount of
// a path it does not share, so the run would otherwise fail somewhere later.
func TestAJobTheContainerCannotReadIsRefusedByName(t *testing.T) {
	// A present binary with a dead daemon is the common shape in CI and in a
	// cloud sandbox, and it is not a failure of this code. Ask the runtime
	// whether it answers, rather than whether it is installed.
	if err := exec.Command(Runtime(), "info").Run(); err != nil {
		t.Skipf("%s is not answering on this machine: %v", Runtime(), err)
	}
	empty := job.Dir(t.TempDir())
	_, err := Run(empty)
	if err == nil {
		t.Fatal("a directory with no job.json must not run")
	}
	if !strings.Contains(err.Error(), "cannot read") || !strings.Contains(err.Error(), empty.Path()) {
		t.Fatalf("the error should name the directory, got %v", err)
	}
}

func TestTheContainerCommandIsYours(t *testing.T) {
	if Runtime() != "docker" {
		t.Fatalf("docker is the default, got %q", Runtime())
	}
	t.Setenv("HAAS_CONTAINER", "podman")
	if Runtime() != "podman" {
		t.Fatalf("HAAS_CONTAINER should choose the runtime, got %q", Runtime())
	}
}

func TestWhatAJobMayTakeIsYoursToSet(t *testing.T) {
	if got := share("HAAS_JOB_MEMORY", "3g"); got != "3g" {
		t.Fatalf("default memory = %q", got)
	}
	t.Setenv("HAAS_JOB_MEMORY", "900m")
	if got := share("HAAS_JOB_MEMORY", "3g"); got != "900m" {
		t.Fatalf("a small machine should be able to say so, got %q", got)
	}
}
