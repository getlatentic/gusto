package runner

import (
	"encoding/json"
	"os"
	"strings"
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

// The line a real opencode run wrote when the agent read a file. Every path in
// the adapter came off this shape, not from a guess at it.
const toolCall = `{"type":"tool_use","sessionID":"ses_x","part":{"type":"tool","tool":"read","callID":"chatcmpl-tool-947b85e34fd7a66d","state":{"status":"completed","input":{"filePath":"/job/workspace/adder.sh"},"title":"adder.sh","time":{"start":1790431302271,"end":1790431302313}}}}`

func TestWhatTheAgentRanIsKeptApartFromWhatItSaid(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	adapter, err := harness.Load("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	record, err := Open(dir, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Out(toolCall); err != nil {
		t.Fatal(err)
	}
	if err := record.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(dir.OutFile("commands.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var ran map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(text))), &ran); err != nil {
		t.Fatal(err)
	}
	if ran["tool"] != "read" || ran["status"] != "completed" || ran["title"] != "adder.sh" {
		t.Fatalf("the record is %v", ran)
	}
	input, ok := ran["input"].(map[string]any)
	if !ok || input["filePath"] != "/job/workspace/adder.sh" {
		t.Fatalf("what it was asked to do is %v", ran["input"])
	}
	// A tool call is not a model call, and counting it as one would double the
	// spend a reader sees.
	if tokens, _ := record.Spent(); tokens != nil {
		t.Fatalf("a tool call was counted as spending %d tokens", *tokens)
	}
}

func TestATranscriptLineIsNotACommand(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	adapter, err := harness.Load("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	record, err := Open(dir, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Out(`{"type":"text","part":{"type":"text","text":"I have read the file."}}`); err != nil {
		t.Fatal(err)
	}
	if err := record.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(dir.OutFile("commands.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(text) != 0 {
		t.Fatalf("commands.jsonl holds %q", text)
	}
}
