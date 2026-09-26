package runner

import (
	"os"
	"testing"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
)

// The line Claude Code ends a run with. Its cache write dwarfs the rest, so a
// total that leaves cache out disagrees with the cost printed beside it.
const result = `{"total_cost_usd":1.3161,"usage":{"input_tokens":12,"cache_creation_input_tokens":39699,"cache_read_input_tokens":6744,"output_tokens":300}}`

func TestASpendTotalCountsTheCacheTokensItWasBilledFor(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	adapter, err := harness.Load("", "claude")
	if err != nil {
		t.Fatal(err)
	}
	record, err := Open(dir, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Out(result); err != nil {
		t.Fatal(err)
	}
	if err := record.Close(); err != nil {
		t.Fatal(err)
	}
	tokens, cost := record.Spent()
	if tokens == nil || cost == nil {
		t.Fatal("a line carrying usage should be counted")
	}
	if want := int64(12 + 39699 + 6744 + 300); *tokens != want {
		t.Errorf("tokens = %d, want %d", *tokens, want)
	}
	if *cost != 1.3161 {
		t.Errorf("cost = %v", *cost)
	}
	if _, err := os.Stat(dir.OutFile("calls.jsonl")); err != nil {
		t.Error("the line should be kept in calls.jsonl")
	}
}
