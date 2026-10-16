package audit_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/runner"
)

// The lines a proxy writes, one of each kind: two model calls that left the
// box, a call a job cap refused, a host the job was let through to, and a host
// it was denied. The last three are not model calls, and their null costs
// must not make the model's cost unknown.
var (
	firstCall   = `{"path":"/v1/chat/completions","status":200,"bytes":1200,"tookMs":40,"credential":"the platform credential","atUnix":101,"model":"m","inputTokens":100,"outputTokens":20,"usd":0.25}`
	secondCall  = `{"path":"/v1/chat/completions","status":200,"bytes":800,"tookMs":30,"credential":"the platform credential","atUnix":102,"model":"m","inputTokens":50,"outputTokens":10,"usd":0.5}`
	refusedCall = `{"path":"/v1/chat/completions","status":402,"tookMs":0,"atUnix":103,"model":"m","inputTokens":0,"outputTokens":0,"usd":0,"refused":{"cap":"job","limitUsd":0.75,"spentUsd":0.75,"reason":"the job has spent its cap"}}`
	reachedHost = `{"path":"","status":200,"bytes":5000,"tookMs":12,"atUnix":104,"inputTokens":null,"outputTokens":null,"usd":null,"host":"registry.npmjs.org:443"}`
	deniedHost  = `{"path":"","status":403,"tookMs":0,"atUnix":105,"inputTokens":null,"outputTokens":null,"usd":null,"host":"exfil.example:443","denied":"not on the allowlist"}`
)

// adapter is a fixture harness that makes the change, prints a line carrying
// something a policy would hide, and leaves the proxy lines where the proxy
// writes them, so the manifest covers them as it covers a real proxy's record.
func adapter(t *testing.T, proxy []string) string {
	t.Helper()
	quoted := make([]string, len(proxy))
	for i, line := range proxy {
		quoted[i] = "'" + line + "'"
	}
	script := "printf yes > answer.txt; echo using key sk-test-0123456789abcdef; printf '%s\\n' " +
		strings.Join(quoted, " ") + " > ../out/proxy.jsonl"
	text, err := json.Marshal(map[string]any{
		"name": "fixture", "program": "sh", "argv": []string{"-c", script},
		"needs": "nothing", "stream": "plain",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

const brief = "make the answer yes for the Acme account"

// ran runs the fixture job, with a line of each kind in its proxy record.
func ran(t *testing.T) job.Dir {
	t.Helper()
	return ranWith(t, firstCall, secondCall, refusedCall, reachedHost, deniedHost)
}

// ranWith runs the fixture job to its gate and gives it the state.json a
// server would have written, so the export has every source it reads.
func ranWith(t *testing.T, proxy ...string) job.Dir {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), adapter(t, proxy))
	t.Setenv("GUSTO_HARNESSES", harnesses)
	t.Setenv("GUSTO_SIGNER", "a-test")

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
