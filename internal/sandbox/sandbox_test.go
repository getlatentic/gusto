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
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("no docker on this machine")
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
