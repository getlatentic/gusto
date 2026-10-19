package runner

import (
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
)

// A turn names no gate, so its harness's exit is what says it failed. A turn
// whose harness never answered must not be recorded as passed.
func TestATurnWithNoGateFailsWhenItsHarnessDid(t *testing.T) {
	for _, tc := range []struct {
		verdict *evidence.Verdict
		exit    int
		want    int
	}{
		{nil, 0, 0},
		{nil, 1, 1},
		{&evidence.Verdict{Passed: true}, 1, 0},
		{&evidence.Verdict{Passed: false}, 0, 1},
	} {
		if got := earned(tc.verdict, tc.exit); got != tc.want {
			t.Errorf("verdict %+v, harness exit %d: earned %d, want %d", tc.verdict, tc.exit, got, tc.want)
		}
	}
}

func TestAServerTheImageDoesNotCarryIsNamedAsMissing(t *testing.T) {
	_, err := located([]job.Server{{Name: "memory", Command: "no-such-mcp-server-anywhere"}})
	if err == nil {
		t.Fatal("a server with no program was accepted")
	}
	found, err := located([]job.Server{{Name: "shell", Command: "sh"}})
	if err != nil || found[0].Command == "sh" || found[0].Command == "" {
		t.Errorf("sh was not resolved to its full path: %+v, %v", found, err)
	}
}
