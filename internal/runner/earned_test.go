package runner

import (
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/job"
)

// A turn names no gate, so its harness's exit and the proxy's word on its model are what say it failed. A turn whose harness or
// model never answered must not be recorded as passed. A gate, when there is one, decides.
func TestATurnWithNoGateFailsWhenItsHarnessOrItsModelDid(t *testing.T) {
	for _, tc := range []struct {
		verdict *evidence.Verdict
		exit    int
		silent  bool
		want    int
	}{
		{nil, 0, false, 0},
		{nil, 1, false, 1},
		{nil, 0, true, 1},
		{&evidence.Verdict{Passed: true}, 1, false, 0},
		{&evidence.Verdict{Passed: true}, 0, true, 0},
		{&evidence.Verdict{Passed: false}, 0, false, 1},
	} {
		if got := earned(tc.verdict, tc.exit, tc.silent); got != tc.want {
			t.Errorf("verdict %+v, harness exit %d, model silent %t: earned %d, want %d", tc.verdict, tc.exit, tc.silent, got, tc.want)
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
