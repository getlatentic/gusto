//go:build !windows

package runner

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

func TestAGateThatNeverEndsIsStoppedWithItsChildrenWhenTheRunEnds(t *testing.T) {
	dir := job.Dir(t.TempDir())
	for _, d := range []string{dir.Workspace(), dir.OutFile("")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The child holds the gate's output open after the shell is killed, as a test runner's workers do.
	spec := job.Spec{Gate: []string{"sh", "-c", "sleep 60 & sleep 60"}}
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	began := time.Now()
	verdict, err := gateWith(ctx, spec, dir, nil, nil, agent{})
	if err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Fatalf("the gate ran for %s after the run ended", took)
	}
	if verdict.Passed {
		t.Fatal("a gate stopped before it answered passed")
	}
}
