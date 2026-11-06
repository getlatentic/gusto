package runner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internal "github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/runner"
)

func TestRefuseKeepsTheReasonWhereAPersonReadingTheTaskLooksAndReturnsIt(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	missing := job.Dir(filepath.Join(t.TempDir(), "no-such-task"))
	_, want := internal.Outside(context.Background(), missing, nil, signing.Identity{})
	if _, err := runner.Outside(context.Background(), missing, nil, runner.Signing{}); want == nil || err == nil || err.Error() != want.Error() {
		t.Errorf("%v, the runner behind it says %v", err, want)
	}
}

func TestAKeyThatCannotBeReadStopsTheRunBeforeTheAgent(t *testing.T) {
	t.Parallel()
	dir := job.Dir(filepath.Join(t.TempDir(), "task"))
	absent := filepath.Join(t.TempDir(), "absent.key")
	if _, err := runner.Outside(context.Background(), dir, nil, runner.Signing{Key: absent}); err == nil || !strings.Contains(err.Error(), "absent.key") {
		t.Fatalf("want the key named, got %v", err)
	}
}
