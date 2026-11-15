package sandbox

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/task"
)

// A directory the container cannot read the task from has to be named as the
// problem. Docker hands over an empty directory rather than refusing a mount of
// a path it does not share, so the run would otherwise fail somewhere later.
func TestATaskTheContainerCannotReadIsRefusedByName(t *testing.T) {
	// A present binary with a dead daemon is the common shape in CI and in a
	// cloud sandbox, and it is not a failure of this code. Ask the runtime
	// whether it answers, rather than whether it is installed.
	c := plain()
	if err := exec.Command(c.Runtime, "info").Run(); err != nil {
		t.Skipf("%s is not answering on this machine: %v", c.Runtime, err)
	}
	// And the image this runs, which a checkout does not come with. Without it
	// the runtime refuses before it ever reads the directory, so the test would
	// fail for a reason that is not the one it is about.
	if err := exec.Command(c.Runtime, "image", "inspect", c.Image).Run(); err != nil {
		t.Skipf("%s is not built on this machine: %s build -t %s .", c.Image, c.Runtime, c.Image)
	}
	empty := task.Dir(t.TempDir())
	_, err := c.Run(context.Background(), empty, "http://localhost:11434/v1", "", Owner{})
	if err == nil {
		t.Fatal("a directory with no job.json must not run")
	}
	if !strings.Contains(err.Error(), "cannot read") || !strings.Contains(err.Error(), empty.Path()) {
		t.Fatalf("the error should name the directory, got %v", err)
	}
}

func TestWhatATaskMayTakeIsYoursToSet(t *testing.T) {
	t.Parallel()
	small := plain()
	small.CPUs, small.Memory = "0.5", "900m"
	if got := strings.Join(small.Limits(), " "); got != "--cpus=0.5 --memory=900m --pids-limit=512" {
		t.Fatalf("a small machine should be able to say so, got %q", got)
	}
}

// Whether the runtime enforces them is measured by the deploy's smoke task, which
// runs a real container. What can be held here is that every container, sealed
// or not, is asked for the same restrictions and no more capabilities than the
// runner needs.
func TestEveryContainerDropsEveryCapabilityButWhatTheRunnerNeeds(t *testing.T) {
	t.Parallel()
	sealed, err := plain().sealedArgv("/jobs/a", "/jobs/.proxy.sock", "t", Owner{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	open, err := plain().argv("/jobs/a", nil, Owner{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{"sealed": sealed, "open": open} {
		if !slices.Contains(args, "--cap-drop=ALL") || !slices.Contains(args, "--security-opt=no-new-privileges") {
			t.Errorf("%s: %v", name, args)
		}
		var added []string
		for _, arg := range args {
			if capability, found := strings.CutPrefix(arg, "--cap-add="); found {
				added = append(added, capability)
			}
		}
		slices.Sort(added)
		if strings.Join(added, ",") != "CHOWN,DAC_OVERRIDE,FOWNER,SETGID,SETUID" {
			t.Errorf("%s adds %v", name, added)
		}
		for _, arg := range args {
			if arg == "--privileged" || strings.HasPrefix(arg, "--device") || strings.HasPrefix(arg, "--cap-add=SYS_") || arg == "--cap-add=NET_ADMIN" {
				t.Errorf("%s was given %q", name, arg)
			}
		}
	}
}
