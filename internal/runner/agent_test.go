package runner

import (
	"os"
	"os/exec"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// A runner that is not root has nobody to hand the work to, so the harness
// runs as the runner and out/ keeps the mode it had: a laptop, or haas inside
// run by hand.
func TestARunnerThatIsNotRootHandsNothingOver(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("the test is about a runner that is not root")
	}
	t.Setenv(AgentUIDEnv, "1000")
	agent := agentHere()
	if agent.present {
		t.Fatal("a runner that is not root found an agent to switch to")
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(dir.Out())
	command := exec.Command("true")
	if err := agent.handOver(dir, command); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(dir.Out())
	if command.SysProcAttr != nil || before.Mode() != after.Mode() {
		t.Errorf("nothing should change: credential %+v, out/ %v then %v", command.SysProcAttr, before.Mode(), after.Mode())
	}
}
