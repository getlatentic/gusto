package runner

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
)

// claudeStream is Claude Code's own JSON stream, as its adapter read it before the agent spoke ACP: a cumulative total
// on the last line, with the cache tokens beside it, and the words in a list of blocks. The record reads any such stream
// the same way, so the shape stays here as the one it is tested with.
func claudeStream() harness.Adapter {
	return harness.Adapter{
		Name: "claude", Program: "claude", Argv: []string{"-p"}, Needs: harness.NeedsKey, Stream: harness.JSONL, Totals: harness.Cumulative,
		When:             map[string]map[string]string{"text": {"type": "assistant"}, "tool": {"type": "assistant"}},
		Text:             "message.content.0.text",
		Message:          "message.id",
		ToolName:         "message.content.0.name",
		ToolInput:        "message.content.0.input",
		SessionID:        "session_id",
		InputTokens:      "usage.input_tokens",
		OutputTokens:     "usage.output_tokens",
		CacheReadTokens:  "usage.cache_read_input_tokens",
		CacheWriteTokens: "usage.cache_creation_input_tokens",
		CostUSD:          "total_cost_usd",
	}
}

// The line Claude Code ends a run with. Its cache write dwarfs the rest, so a
// total that leaves cache out disagrees with the cost printed beside it.
const result = `{"total_cost_usd":1.3161,"usage":{"input_tokens":12,"cache_creation_input_tokens":39699,"cache_read_input_tokens":6744,"output_tokens":300}}`

func TestASpendTotalCountsTheCacheTokensItWasBilledFor(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	record, err := Open(dir, claudeStream())
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

// A harness that counts its tokens and names no price is the ordinary case,
// and a cost of zero written there would say the run was free. claude reported
// $0.28 of Anthropic prices for tokens another provider served, so its price
// was dropped from the adapter — and the record then said the run cost nothing.
func TestTokensWithoutAPriceAreNotARunThatWasFree(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	record, err := Open(dir, harness.Adapter{
		Stream: harness.JSONL, Totals: harness.PerCall,
		InputTokens: "usage.input", OutputTokens: "usage.output",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Out(`{"usage":{"input":120,"output":30}}`); err != nil {
		t.Fatal(err)
	}
	tokens, cost := record.Spent()
	if tokens == nil || *tokens != 150 {
		t.Fatalf("tokens read as %v", tokens)
	}
	if cost != nil {
		t.Errorf("a run nobody priced was recorded as costing %v", *cost)
	}
}

// Lines dsh sent over ACP while fixing a file, and a prompt's answer carrying
// the usage the protocol defines.
var acpTurn = []string{
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"Let me look at the file."}}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","messageId":"m1","content":{"type":"text","text":"I'll look"}}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","messageId":"m1","content":{"type":"text","text":" first."}}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"usage_update","used":9426,"size":1000000}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call","toolCallId":"c1","title":"edit","kind":"other","status":"in_progress","rawInput":{"file_path":"adder.sh"}}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"tool_call_update","toolCallId":"c1","status":"completed","content":[{"type":"content","content":{"type":"text","text":"updated"}}]}}}`,
	`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"s1","update":{"sessionUpdate":"agent_message_chunk","messageId":"m2","content":{"type":"text","text":"Fixed."}}}}`,
	`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn","usage":{"totalTokens":1000,"inputTokens":800,"outputTokens":200,"cachedReadTokens":600}}}`,
}

func TestAnACPHarnessIsReadWhereTheProtocolSaysThings(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	adapter, err := harness.Load("", "dsh")
	if err != nil {
		t.Fatal(err)
	}
	record, err := Open(dir, adapter)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range acpTurn {
		if err := record.Out(line); err != nil {
			t.Fatal(err)
		}
	}
	tokens, cost := record.Spent()
	if err := record.Close(); err != nil {
		t.Fatal(err)
	}
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	if want := "I'll look first.\n\nFixed."; string(said) != want {
		t.Errorf("transcript = %q, want %q: each message its own paragraph, and no thinking", said, want)
	}
	ran, _ := os.ReadFile(dir.OutFile("commands.jsonl"))
	if lines := strings.Split(strings.TrimSpace(string(ran)), "\n"); len(lines) != 1 || !strings.Contains(lines[0], `"tool":"edit"`) {
		t.Errorf("commands = %s, want the one call, once", ran)
	}
	if tokens == nil || *tokens != 1000 {
		t.Errorf("tokens = %v, want the total the harness stated", tokens)
	}
	if cost != nil {
		t.Error("no cost was named, so none is claimed")
	}
}

// claudeTurn is Claude Code's stream as it came from a turn that loaded a
// skill: the skill arrives on a user line, which is not the agent speaking.
var claudeTurn = []string{
	`{"type":"system","subtype":"init","session_id":"s1"}`,
	`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"thinking","thinking":"read it first"}]}}`,
	`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"text","text":"I'll read the notes first."}]}}`,
	`{"type":"assistant","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"c1","name":"Skill","input":{"skill":"soap-note"}}]}}`,
	`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Base directory for this skill: /root/.local/share/claude/skills/soap-note\n\nWrite the note."}]}}`,
	`{"type":"assistant","message":{"id":"m2","role":"assistant","content":[{"type":"text","text":"Drafted to soap-note.md."}]}}`,
	`{"type":"result","subtype":"success","session_id":"s1","usage":{"input_tokens":800,"output_tokens":200}}`,
}

func TestClaudeCodeIsHeardOnlyWhereTheAgentSpeaks(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	record, err := Open(dir, claudeStream())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range claudeTurn {
		if err := record.Out(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := record.Close(); err != nil {
		t.Fatal(err)
	}
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	if want := "I'll read the notes first.\n\nDrafted to soap-note.md."; string(said) != want {
		t.Errorf("transcript = %q, want %q: the agent's words only, a paragraph a message", said, want)
	}
}

// A tool call says how much of the transcript was written when it ran, so that what the agent said can be shown around it.
func TestAToolCallSaysWhereInTheTranscriptItRan(t *testing.T) {
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
	line, err := os.ReadFile(dir.OutFile("commands.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var kept struct {
		At *int64 `json:"at"`
	}
	if json.Unmarshal(line, &kept) != nil || kept.At == nil || *kept.At != 0 {
		t.Errorf("the call did not say it ran at the start of the transcript: %s", line)
	}
}
