package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/runner"
)

// A job run in the server's own process is given an environment of its own: the harness and the gate see what they were given
// and nothing of the process they run in, where the server keeps its secrets.
func TestAHarnessAndItsGateRunWithTheEnvironmentGivenAndNotTheServers(t *testing.T) {
	t.Setenv("HAAS_PASSWORD", "the-servers-own-secret")
	mine := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mine, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(mine, "plain", "adapter.json"), `{"name": "plain", "program": "sh", "needs": "nothing",
		"argv": ["-c", "printf 'token=%s password=%s' \"$HAAS_MODEL_TOKEN\" \"${HAAS_PASSWORD:-absent}\" > seen.txt; printf yes > answer.txt"]}`)
	t.Setenv("HAAS_HARNESSES", mine)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes && test -z "${HAAS_PASSWORD:-}" && test "$HAAS_MODEL_TOKEN" = given-token`},
		Harness: "plain",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	env := harness.Env{
		Base:       []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "HAAS_MODEL_URL=http://127.0.0.1:1/model/v1", "HAAS_MODEL_TOKEN=given-token"},
		ModelURL:   "http://127.0.0.1:1/model/v1",
		ModelToken: "given-token",
	}
	exit, err := runner.InsideWith(context.Background(), dir, env)
	if err != nil || exit != 0 {
		gate, _ := os.ReadFile(dir.OutFile("gate.log"))
		t.Fatalf("exit %d, %v: the gate saw the server's environment or no token\n%s", exit, err, gate)
	}
	seen, _ := os.ReadFile(filepath.Join(dir.Workspace(), "seen.txt"))
	if string(seen) != "token=given-token password=absent" {
		t.Errorf("the harness saw %q", seen)
	}
	if strings.Contains(string(seen), "the-servers-own-secret") {
		t.Fatal("the server's secret reached the harness")
	}
}
