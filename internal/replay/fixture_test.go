package replay_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/job"
)

// Harnesses that are programs. One leaves the answer the gate wants, one leaves
// an answer it refuses, and one leaves the right answer and commits it, which
// moves the workspace's HEAD off the commit the job started from.
const (
	answersYes  = `{"name": "yes", "program": "sh", "argv": ["-c", "printf yes > answer.txt"], "needs": "nothing", "stream": "plain"}`
	answersNo   = `{"name": "maybe", "program": "sh", "argv": ["-c", "printf maybe > answer.txt"], "needs": "nothing", "stream": "plain"}`
	commitsYes  = `{"name": "committer", "program": "sh", "argv": ["-c", "printf yes > answer.txt && git -c user.email=h@example.com -c user.name=H commit --quiet -am done"], "needs": "nothing", "stream": "plain"}`
	regressedTo = `{"name": "yes", "program": "sh", "argv": ["-c", "printf maybe > answer.txt"], "needs": "nothing", "stream": "plain"}`
)

// hermetic keeps the machine's own git configuration out of every git the test
// runs, and puts the fixture adapters where the runner looks.
func hermetic(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GUSTO_SIGNER", "a-test")
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "yes.json"), answersYes)
	write(t, filepath.Join(harnesses, "maybe.json"), answersNo)
	write(t, filepath.Join(harnesses, "committer.json"), commitsYes)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	return harnesses
}

// remote is a bare repository on main whose answer is no.
func remote(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	write(t, filepath.Join(source, "answer.txt"), "no")
	git(t, source, "init", "--quiet", "-b", "main")
	git(t, source, "add", "answer.txt")
	commit(t, source, "start")
	bare := filepath.Join(t.TempDir(), "remote.git")
	git(t, source, "clone", "--quiet", "--bare", source, bare)
	return bare
}

// advance puts a new commit on the remote's main, so a job cloning HEAD now
// would start somewhere else.
func advance(t *testing.T, bare string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	git(t, t.TempDir(), "clone", "--quiet", bare, work)
	write(t, filepath.Join(work, "later.txt"), "after the run")
	git(t, work, "add", "later.txt")
	commit(t, work, "later")
	git(t, work, "push", "--quiet", "origin", "main")
	return git(t, work, "rev-parse", "HEAD")
}

// finished runs a whole job with the named harness under parent.
func finished(t *testing.T, parent, name, harness, bare string) job.Dir {
	t.Helper()
	dir := job.Dir(filepath.Join(parent, name))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    bare,
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: harness,
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Inside(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func commit(t *testing.T, at, message string) {
	t.Helper()
	git(t, at, "-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--quiet", "-m", message)
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
