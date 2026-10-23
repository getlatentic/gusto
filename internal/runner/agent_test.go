package runner

import (
	"os"
	"os/exec"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

// A runner that is not root has nobody to hand the work to, so the harness
// runs as the runner and out/ keeps the mode it had: a laptop, or gusto inside
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

// A runner that has nobody to hand the work to has nothing to take back.
func TestARunnerWithNoAgentTakesBackNothing(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	file := dir.Workspace() + "/made-by-the-harness"
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(file)
	agent{}.takeBack(dir)
	after, _ := os.Stat(file)
	if before.Mode() != after.Mode() {
		t.Errorf("a file changed: %v then %v", before.Mode(), after.Mode())
	}
}
