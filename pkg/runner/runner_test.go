package runner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internal "github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/runner"
)

func TestRefuseKeepsTheReasonWhereAPersonReadingTheTaskLooksAndReturnsIt(t *testing.T) {
	dir := job.Dir(filepath.Join(t.TempDir(), "task"))
	reason := errors.New("the model is not allowed")
	if got := runner.Refuse(dir, reason); !errors.Is(got, reason) {
		t.Fatalf("returned %v", got)
	}
	said, err := os.ReadFile(dir.OutFile("transcript.log"))
	if err != nil || !strings.Contains(string(said), "refused this task before the agent started: the model is not allowed") {
		t.Fatalf("%v %q", err, said)
	}
}

func TestTheFacadeAnswersAsTheRunnerBehindItDoes(t *testing.T) {
	missing := job.Dir(filepath.Join(t.TempDir(), "no-such-task"))
	_, want := internal.OutsideContext(context.Background(), missing, nil)
	for name, got := range map[string]func() error{
		"Outside":        func() error { _, err := runner.Outside(missing, nil); return err },
		"OutsideContext": func() error { _, err := runner.OutsideContext(context.Background(), missing, nil); return err },
	} {
		if err := got(); want == nil || err == nil || err.Error() != want.Error() {
			t.Errorf("%s: %v, the runner behind it says %v", name, err, want)
		}
	}
}
