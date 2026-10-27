package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

func TestExplainFailureWritesTheReasonWhenNothingWasRecorded(t *testing.T) {
	dir := job.Dir(t.TempDir())
	explainFailure(dir, -1, errors.New("pi: no model"))
	got, err := os.ReadFile(dir.OutFile("stderr.log"))
	if err != nil || !strings.Contains(string(got), "pi: no model") {
		t.Fatalf("stderr.log = %q, %v", got, err)
	}
}

func TestExplainFailureNamesTheStatusWhenThereIsNoError(t *testing.T) {
	dir := job.Dir(t.TempDir())
	explainFailure(dir, 3, nil)
	got, _ := os.ReadFile(dir.OutFile("stderr.log"))
	if !strings.Contains(string(got), "status 3") {
		t.Fatalf("stderr.log = %q", got)
	}
}

func TestExplainFailureLeavesARunThatSpokeAlone(t *testing.T) {
	for _, file := range []string{"transcript.log", "stderr.log", "manifest.json"} {
		dir := job.Dir(t.TempDir())
		if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir.Out(), file), []byte("its own words\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		explainFailure(dir, 1, errors.New("late"))
		got, _ := os.ReadFile(dir.OutFile(file))
		if string(got) != "its own words\n" {
			t.Fatalf("%s was changed to %q", file, got)
		}
		if file != "stderr.log" {
			if _, err := os.Stat(dir.OutFile("stderr.log")); err == nil {
				t.Fatalf("stderr.log was written beside %s", file)
			}
		}
	}
}
