package compare_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

// Harnesses that are programs: one leaves the answer the gate wants, one leaves
// an answer it refuses, and one names a program nobody has.
const (
	answersYes = `{"name": "yes", "program": "sh", "argv": ["-c", "printf yes > answer.txt"], "needs": "nothing", "stream": "plain"}`
	answersNo  = `{"name": "maybe", "program": "sh", "argv": ["-c", "printf maybe > answer.txt"], "needs": "nothing", "stream": "plain"}`
	missing    = `{"name": "missing", "program": "no-such-program-anywhere", "argv": [], "needs": "nothing", "stream": "plain"}`
)

func hermetic(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GUSTO_SIGNER", "a-test")
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "yes.json"), answersYes)
	write(t, filepath.Join(harnesses, "maybe.json"), answersNo)
	write(t, filepath.Join(harnesses, "missing.json"), missing)
	t.Setenv("GUSTO_HARNESSES", harnesses)
}

// remote is a bare repository on main whose answer is no.
func remote(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	write(t, filepath.Join(source, "answer.txt"), "no")
	git(t, source, "init", "--quiet", "-b", "main")
	git(t, source, "add", "answer.txt")
	git(t, source, "-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--quiet", "-m", "start")
	bare := filepath.Join(t.TempDir(), "remote.git")
	git(t, source, "clone", "--quiet", "--bare", source, bare)
	return bare
}

// advance puts a new commit on the remote's main.
func advance(t *testing.T, bare string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	git(t, t.TempDir(), "clone", "--quiet", bare, work)
	write(t, filepath.Join(work, "later.txt"), "after the pin")
	git(t, work, "add", "later.txt")
	git(t, work, "-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--quiet", "-m", "later")
	git(t, work, "push", "--quiet", "origin", "main")
}

// submitted is a job as `gusto new` leaves it: a job.json and nothing run.
func submitted(t *testing.T, bare string, model string) job.Dir {
	t.Helper()
	dir := job.Dir(filepath.Join(t.TempDir(), "fix"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    bare,
		Ref:     "main",
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "yes",
		Model:   model,
		Budget:  job.Budget{Seconds: 60},
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func git(t *testing.T, at string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = at
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}
