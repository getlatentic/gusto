package matrix_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/matrix"
	"github.com/getlatentic/gusto/internal/replay"
	"github.com/getlatentic/gusto/pkg/job"
)

// Harnesses that are programs, so no model is asked anything. "says" writes the
// model it was given as the answer, so the model decides whether the gate
// passes: says@yes finishes and says@no does not. "priced" finishes and reports
// what it cost the way a harness prints it. "missing" names a program nobody
// has, so it never leaves a verdict.
const (
	says    = `{"name": "says", "program": "sh", "argv": ["-c", "printf %s \"$0\" > answer.txt", "{model}"], "needs": "nothing", "stream": "plain"}`
	priced  = `{"name": "priced", "program": "sh", "argv": ["-c", "printf yes > answer.txt; echo '{\"in\": 90, \"out\": 10, \"cost\": 0.25}'"], "needs": "nothing", "stream": "jsonl", "totals": "perCall", "inputTokens": "in", "outputTokens": "out", "costUsd": "cost"}`
	missing = `{"name": "missing", "program": "no-such-program-anywhere", "argv": [], "needs": "nothing", "stream": "plain"}`
)

var (
	finishes      = matrix.Pair{Harness: "says", Model: "yes"}
	doesNotFinish = matrix.Pair{Harness: "says", Model: "no"}
)

func hermetic(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GUSTO_SIGNER", "a-test")
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "says.json"), says)
	write(t, filepath.Join(harnesses, "priced.json"), priced)
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

// submitted is a job as `gusto new` leaves it, named name, under at.
func submitted(t *testing.T, at, name, bare string) string {
	t.Helper()
	dir := job.Dir(filepath.Join(at, name))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    bare,
		Ref:     "main",
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "says",
		Budget:  job.Budget{Seconds: 60},
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	return dir.Path()
}

// planned writes a plan beside two jobs, the first the simplest, and returns
// its path.
func planned(t *testing.T, bare string, pairs ...matrix.Pair) string {
	t.Helper()
	at := t.TempDir()
	submitted(t, at, "one-line-fix", bare)
	submitted(t, at, "two-line-fix", bare)
	plan := matrix.Plan{Environment: "a test", Jobs: []string{"one-line-fix", "two-line-fix"}, Pairs: pairs}
	text, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(at, "plan.json")
	write(t, path, string(text))
	return path
}

// counting runs every cell here and counts the runs, since each one costs.
func counting(ran *int) matrix.Way {
	return matrix.Way{
		Where: "here, in this process",
		Open: func(string) (replay.Run, func(), error) {
			return func(dir job.Dir) error { *ran++; return replay.Here(dir) }, func() {}, nil
		},
	}
}

func measure(t *testing.T, path, again string, way matrix.Way) (string, matrix.Table) {
	t.Helper()
	plan, err := matrix.ReadPlan(path)
	if err != nil {
		t.Fatal(err)
	}
	where := matrix.Beside(path)
	if _, err := matrix.Measure(plan, where, way, matrix.AgainFor(again), &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	table, err := matrix.Tabulate(where)
	if err != nil {
		t.Fatal(err)
	}
	return where, table
}

func rowFor(t *testing.T, table matrix.Table, job string, pair matrix.Pair) matrix.Row {
	t.Helper()
	for _, row := range table.Rows {
		if row.Job == job && row.Harness == pair.Harness && row.Model == pair.Model {
			return row
		}
	}
	t.Fatalf("no row for %s on %s in %+v", job, pair.Name(), table.Rows)
	return matrix.Row{}
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
