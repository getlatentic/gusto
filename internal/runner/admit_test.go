package runner_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/runner"
)

// absent names a program no machine has, which is what a job naming a harness
// the image was not built with looks like from inside the box.
const absent = `{
  "name": "absent",
  "program": "gusto-fixture-harness-nobody-installed",
  "argv": ["{brief}"],
  "needs": "nothing",
  "stream": "plain"
}`

func refused(t *testing.T) (job.Dir, error) {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "absent.json"), absent)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "absent",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	_, err := runner.Inside(dir)
	return dir, err
}

func TestAHarnessTheRunnerLacksIsRefusedBeforeItStarts(t *testing.T) {
	dir, err := refused(t)
	if err == nil {
		t.Fatal("a job whose harness is not here should be refused")
	}
	for _, said := range []string{"does not carry gusto-fixture-harness-nobody-installed", "absent in its HARNESSES"} {
		if !strings.Contains(err.Error(), said) {
			t.Errorf("the refusal says %q, and should say %q", err, said)
		}
	}
	// Nothing the harness would have written exists, and no manifest claims a
	// run happened: the job did not start, rather than started and failed.
	for _, name := range []string{"harness.log", "stderr.log", "manifest.json"} {
		if _, err := os.Stat(dir.OutFile(name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s exists for a job that never started", name)
		}
	}
}

func TestTheRefusalIsWhereSomebodyReadingTheJobLooks(t *testing.T) {
	dir, err := refused(t)
	transcript, readErr := os.ReadFile(dir.OutFile("transcript.log"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(transcript), "refused this job before the harness started") ||
		!strings.Contains(string(transcript), err.Error()) {
		t.Fatalf("the transcript says %q", transcript)
	}
}
