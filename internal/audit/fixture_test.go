package audit_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/runner"
)

// The fixture harness makes the change, prints a line carrying something a
// policy would hide, and leaves two proxy lines where the proxy writes them,
// so the manifest covers them as it covers a real proxy's record.
const adapter = `{
  "name": "fixture",
  "program": "sh",
  "argv": ["-c", "printf yes > answer.txt; echo using key sk-test-0123456789abcdef; printf '%s\\n' '{\"path\":\"/v1/chat/completions\",\"status\":200,\"bytes\":1200,\"tookMs\":40,\"credential\":\"the platform credential\"}' '{\"path\":\"/v1/chat/completions\",\"status\":200,\"bytes\":800,\"tookMs\":30,\"credential\":\"the platform credential\"}' > ../out/proxy.jsonl"],
  "needs": "nothing",
  "stream": "plain"
}`

const brief = "make the answer yes for the Acme account"

// ran runs the fixture job to its gate and gives it the state.json a server
// would have written, so the export has every source it reads.
func ran(t *testing.T) job.Dir {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), adapter)
	t.Setenv("HAAS_HARNESSES", harnesses)
	t.Setenv("HAAS_SIGNER", "a-test")

	dir := job.Dir(filepath.Join(t.TempDir(), "20260926-0900-answer"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   brief,
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "fixture",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if exit, err := runner.Inside(dir); err != nil || exit != 0 {
		t.Fatalf("the fixture should pass, exit %d: %v", exit, err)
	}
	state, err := json.MarshalIndent(jobs.Record{
		ID: filepath.Base(dir.Path()), State: jobs.Passed, Brief: brief, Repo: spec.Repo,
		Harness: "fixture", Asked: "the shared sign-in", Queued: 100, Started: 101, Ended: 120,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir.Path(), "state.json"), string(state))
	return dir
}

// repo is a checkout whose gate fails until answer.txt says yes.
func repo(t *testing.T) string {
	t.Helper()
	at := t.TempDir()
	write(t, filepath.Join(at, "answer.txt"), "no")
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"add", "answer.txt"},
		{"-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--quiet", "-m", "start"},
	} {
		command := exec.Command("git", args...)
		command.Dir = at
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return at
}

func write(t *testing.T, file, text string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, file string) string {
	t.Helper()
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}
