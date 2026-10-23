package ship_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/job"
)

// Two harnesses that are programs: one leaves the answer the gate wants, and
// one leaves an answer it refuses.
const (
	answersYes = `{"name": "yes", "program": "sh", "argv": ["-c", "printf yes > answer.txt"], "needs": "nothing", "stream": "plain"}`
	answersNo  = `{"name": "maybe", "program": "sh", "argv": ["-c", "printf maybe > answer.txt"], "needs": "nothing", "stream": "plain"}`
)

// hermetic keeps the machine's own git configuration, signing included, out of
// every git the test runs.
func hermetic(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GUSTO_SIGNER", "a-test")
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "yes.json"), answersYes)
	write(t, filepath.Join(harnesses, "maybe.json"), answersNo)
	t.Setenv("GUSTO_HARNESSES", harnesses)
}

// remote is a bare repository on main whose answer is no. It is where the job
// clones from and where shipping pushes to.
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

// finished runs a whole job with the named harness and returns its directory
// and the remote it came from.
func finished(t *testing.T, harness string) (job.Dir, string) {
	t.Helper()
	hermetic(t)
	bare := remote(t)
	dir := job.Dir(filepath.Join(t.TempDir(), "20260926-0900-make-the-answer-yes"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    bare,
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: harness,
		Model:   "local/fixture-model",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Inside(dir); err != nil {
		t.Fatal(err)
	}
	return dir, bare
}

// forge stands in for GitHub and keeps every request it was sent.
type forge struct {
	mu   sync.Mutex
	seen []sent
}

type sent struct {
	Method string
	Path   string
	Body   map[string]any
}

func (f *forge) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		text, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(text, &body)
		f.mu.Lock()
		f.seen = append(f.seen, sent{Method: r.Method, Path: r.URL.Path, Body: body})
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"html_url": "https://github.com/o/r/pull/1"}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func (f *forge) requests() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent(nil), f.seen...)
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
