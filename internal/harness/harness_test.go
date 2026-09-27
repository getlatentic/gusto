package harness

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

func TestTheAdaptersThatShipParse(t *testing.T) {
	t.Setenv("HAAS_HARNESSES", "")
	names := Names()
	if len(names) == 0 {
		t.Fatal("no adapter ships with this build")
	}
	for _, name := range names {
		adapter, err := Load("", name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if adapter.Name != name || adapter.Program == "" || len(adapter.Argv) == 0 {
			t.Errorf("%s is not runnable: %+v", name, adapter)
		}
	}
}

func TestAJobsValuesReachTheCommandLine(t *testing.T) {
	adapter, err := Load("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "fix the adder", Model: "ollama/gpt-oss:20b"}
	command := adapter.Command(context.Background(), spec, t.TempDir())
	if !slices.Contains(command.Args, "--model=ollama/gpt-oss:20b") {
		t.Errorf("the model should reach the command line: %v", command.Args)
	}
	if !slices.Contains(command.Args, "fix the adder") {
		t.Errorf("the brief should reach the command line: %v", command.Args)
	}
}

func TestAnArgumentTheJobLeftEmptyIsDropped(t *testing.T) {
	adapter, err := Load("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	command := adapter.Command(context.Background(), job.Spec{Brief: "b"}, t.TempDir())
	for _, argument := range command.Args {
		if strings.HasSuffix(argument, "=") {
			t.Fatalf("a bare flag reached the command line: %v", command.Args)
		}
	}
}

// The paths in the opencode adapter were read off a real run of opencode
// 1.18.10 against a local model. This is that line.
const stepFinish = `{"type":"step_finish","part":{"type":"step-finish","cost":0,"tokens":{"total":17387,"input":17385,"output":2,"reasoning":0}}}`

func TestTheOpencodeAdapterFindsTheNumbersInARealLine(t *testing.T) {
	adapter, err := Load("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(stepFinish), &line); err != nil {
		t.Fatal(err)
	}
	input, ok := Number(line, adapter.InputTokens)
	if !ok || input != 17385 {
		t.Errorf("input tokens: %v %t", input, ok)
	}
	output, ok := Number(line, adapter.OutputTokens)
	if !ok || output != 2 {
		t.Errorf("output tokens: %v %t", output, ok)
	}
	if _, ok := Number(line, adapter.CostUSD); !ok {
		t.Error("cost should be found, even at zero")
	}
}

func TestAnAdapterOfYourOwnWinsOverABuiltInOne(t *testing.T) {
	mine := t.TempDir()
	if err := writeFile(mine+"/opencode.json", `{"name":"opencode","program":"mine","argv":["go"],"needs":"nothing"}`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HAAS_HARNESSES", mine)
	adapter, err := Load("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Program != "mine" {
		t.Fatalf("want the local adapter, got %q", adapter.Program)
	}
}

func writeFile(path, text string) error {
	return os.WriteFile(path, []byte(text), 0o644)
}

func TestAnAdapterCarriedByTheJobIsFoundWithNothingElseSet(t *testing.T) {
	t.Setenv("HAAS_HARNESSES", "")
	dir := job.Dir(t.TempDir())
	if err := os.MkdirAll(Carried(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := `{"name":"carried","program":"sh","argv":["-c","true"],"needs":"nothing"}`
	if err := writeFile(Carried(dir)+"/carried.json", mine); err != nil {
		t.Fatal(err)
	}
	adapter, err := Load(dir, "carried")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Program != "sh" {
		t.Fatalf("want the carried adapter, got %+v", adapter)
	}
}

func TestANewJobKeepsAnAdapterOfYourOwnButNotABuiltInOne(t *testing.T) {
	mine := t.TempDir()
	if err := writeFile(mine+"/mine.json", `{"name":"mine","program":"sh","argv":["-c","true"],"needs":"nothing"}`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HAAS_HARNESSES", mine)
	dir := job.Dir(t.TempDir())
	if err := Keep(dir, "mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Carried(dir) + "/mine.json"); err != nil {
		t.Fatalf("an adapter of your own should travel: %v", err)
	}
	if err := Keep(dir, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Carried(dir) + "/opencode.json"); err == nil {
		t.Fatal("a built-in adapter is already everywhere and should not be copied")
	}
}

// Read off a real run of Claude Code: the text arrives as a list of blocks, and
// the totals arrive on the last line.
const claudeText = `{"type":"assistant","message":{"model":"claude-opus-5","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":2,"output_tokens":4}}}`
const claudeResult = `{"duration_api_ms":1733,"stop_reason":"end_turn","total_cost_usd":0.39709999999999995,"usage":{"input_tokens":2,"cache_creation_input_tokens":39699,"output_tokens":4}}`

func TestTheClaudeAdapterReadsALineWithAListInIt(t *testing.T) {
	adapter, err := Load("", "claude")
	if err != nil {
		t.Fatal(err)
	}
	var said map[string]any
	if err := json.Unmarshal([]byte(claudeText), &said); err != nil {
		t.Fatal(err)
	}
	value, ok := Pick(said, adapter.Text)
	if !ok || value != "ok" {
		t.Fatalf("text through a list: %v %t", value, ok)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(claudeResult), &result); err != nil {
		t.Fatal(err)
	}
	cost, ok := Number(result, adapter.CostUSD)
	if !ok || cost < 0.39 || cost > 0.40 {
		t.Fatalf("cost: %v %t", cost, ok)
	}
	if adapter.Totals != Cumulative {
		t.Fatal("Claude Code reports a running total, so summing would count every call again")
	}
}

func TestAPathIntoAListThatIsNotThereIsNotFound(t *testing.T) {
	var said map[string]any
	if err := json.Unmarshal([]byte(claudeText), &said); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"message.content.9.text", "message.content.x.text", "message.usage.0"} {
		if _, ok := Pick(said, path); ok {
			t.Errorf("%s should not resolve", path)
		}
	}
}

func TestWhatEachHarnessTalksTo(t *testing.T) {
	t.Setenv("HAAS_MODEL_URL", "http://localhost:11434/v1")
	for name, want := range map[string]string{
		"opencode": "http://localhost:11434/v1",
		"claude":   "https://api.anthropic.com",
		"codex":    "https://api.openai.com",
	} {
		adapter, err := Load("", name)
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.EndpointOf(); got != want {
			t.Errorf("%s talks to %q, want %q", name, got, want)
		}
	}
	program := Adapter{Needs: NeedsNothing}
	if got := program.EndpointOf(); got != "" {
		t.Errorf("a harness needing no model talks to %q", got)
	}
}

func TestTheNamesOfferedIncludeYourOwn(t *testing.T) {
	mine := t.TempDir()
	if err := writeFile(mine+"/fixture.json", `{"name":"fixture","program":"sh","argv":["-c","true"],"needs":"nothing"}`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HAAS_HARNESSES", mine)
	names := Names()
	if !slices.Contains(names, "fixture") || !slices.Contains(names, "claude") {
		t.Fatalf("want both a harness of my own and the built-in ones, got %v", names)
	}
	if !slices.IsSorted(names) {
		t.Fatalf("a list a person reads should be sorted: %v", names)
	}
}

// Some harnesses take the brief as an argument and some read it. codex waits on
// standard input forever when it is given neither, which in a container is a
// job that never ends.
func TestAHarnessThatReadsItsBriefIsGivenIt(t *testing.T) {
	adapter, err := Load("", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Stdin == "" {
		t.Fatal("codex reads its brief on standard input and the adapter does not say so")
	}
	command := adapter.Command(context.Background(), job.Spec{Brief: "fix the adder"}, t.TempDir())
	if command.Stdin == nil {
		t.Fatal("nothing was put on standard input")
	}
	given, err := io.ReadAll(command.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	if string(given) != "fix the adder" {
		t.Fatalf("standard input carried %q", given)
	}
	for _, argument := range command.Args {
		if argument == "fix the adder" {
			t.Fatal("the brief was also passed as an argument, which codex would read as a path")
		}
	}
}

// Read off a real codex run with gpt-5.5. The text arrives as a completed item
// and the usage on the turn that finished; codex reports no cost at all.
const codexText = `{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"ok"}}`
const codexUsage = `{"type":"turn.completed","usage":{"input_tokens":21768,"cached_input_tokens":1408,"cache_write_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0}}`
const codexThread = `{"type":"thread.started","thread_id":"01a0de23-bcfd-7220-8282-bba4fac27745"}`

func TestTheCodexAdapterReadsARealRun(t *testing.T) {
	adapter, err := Load("", "codex")
	if err != nil {
		t.Fatal(err)
	}
	said := decode(t, codexText)
	if value, ok := Pick(said, adapter.Text); !ok || value != "ok" {
		t.Fatalf("text: %v %t", value, ok)
	}
	// An error item carries a message rather than text, so the text path does
	// not mistake a warning for something the agent said.
	warning := decode(t, `{"type":"item.completed","item":{"type":"error","message":"skills were shortened"}}`)
	if _, ok := Pick(warning, adapter.Text); ok {
		t.Fatal("an error item read as something the agent said")
	}

	usage := decode(t, codexUsage)
	for path, want := range map[string]float64{
		adapter.InputTokens:      21768,
		adapter.OutputTokens:     5,
		adapter.CacheReadTokens:  1408,
		adapter.CacheWriteTokens: 0,
	} {
		if got, ok := Number(usage, path); !ok || got != want {
			t.Errorf("%s = %v (%t), want %v", path, got, ok, want)
		}
	}
	if adapter.CostUSD != "" {
		t.Fatal("codex reports no cost, and inventing one would be a number nobody measured")
	}

	thread := decode(t, codexThread)
	if value, ok := Pick(thread, adapter.SessionID); !ok || value != "01a0de23-bcfd-7220-8282-bba4fac27745" {
		t.Fatalf("the conversation's name: %v %t", value, ok)
	}
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

// Codex names no tool. A shell call is an item carrying a command, and nothing
// else in its stream has that field — which is what tells a call from a message.
// Both lines here are real, from a run against deepseek-flash.
func TestCodexShellCallsAreToldApartFromItsMessages(t *testing.T) {
	adapter, err := Load("", "codex")
	if err != nil {
		t.Fatal(err)
	}
	ran := `{"type":"item.completed","item":{"id":"item_2","type":"command_execution",` +
		`"command":"/bin/zsh -lc 'cat adder.sh'","status":"completed"}}`
	said := `{"type":"item.completed","item":{"id":"item_4","type":"agent_message",` +
		`"text":"Root cause: the arithmetic operator is - instead of +."}}`

	var call, message map[string]any
	if err := json.Unmarshal([]byte(ran), &call); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(said), &message); err != nil {
		t.Fatal(err)
	}
	name, isCall := Pick(call, adapter.ToolName)
	if ran, _ := name.(string); !isCall || !strings.Contains(ran, "cat adder.sh") {
		t.Errorf("a shell call read as %q, %v", name, isCall)
	}
	if status, found := Pick(call, adapter.ToolStatus); !found || status != "completed" {
		t.Errorf("its status read as %q, %v", status, found)
	}
	if _, isCall := Pick(message, adapter.ToolName); isCall {
		t.Error("what the harness said was recorded as a command it ran")
	}
	text, found := Pick(message, adapter.Text)
	if said, _ := text.(string); !found || !strings.Contains(said, "Root cause") {
		t.Errorf("what it said read as %q, %v", text, found)
	}
}

// pi reports one message's usage on message_update, message_end and turn_end
// alike. Counted on every line that carries numbers, a run that spent 1,194
// input tokens reported 3,193; counted on the one line the adapter names, it
// reports what it spent. Every line here is real, from a run against
// deepseek-flash.
func TestUsageIsTakenOnlyFromTheLinesAnAdapterNames(t *testing.T) {
	adapter, err := Load("", "pi")
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]string{
		"message_end":    `{"type":"message_end","message":{"usage":{"input":157,"output":26}}}`,
		"turn_end":       `{"type":"turn_end","message":{"usage":{"input":157,"output":26}}}`,
		"message_update": `{"type":"message_update","message":{"usage":{"input":140,"output":20}},"assistantMessageEvent":{"delta":"ok"}}`,
	}
	for kind, text := range lines {
		var line map[string]any
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			t.Fatal(err)
		}
		if counts := adapter.Counts(line); counts != (kind == "message_end") {
			t.Errorf("%s counts=%v", kind, counts)
		}
		// The numbers are readable on every one of them, which is the trap.
		if _, found := Number(line, adapter.InputTokens); !found {
			t.Errorf("%s: the numbers should be readable, and are not", kind)
		}
	}
}

// An adapter that narrows nothing counts every line carrying numbers, which is
// what every adapter but pi does.
func TestAnAdapterThatNarrowsNothingCountsEveryLine(t *testing.T) {
	adapter, err := Load("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.Counts(map[string]any{"anything": "at all"}) {
		t.Error("an adapter naming no lines refused one")
	}
}

// An Anthropic-shaped client adds /v1/messages itself, so it is handed the
// address without a version on it. Handed the /v1 form it asked the provider
// for /v1/v1/messages and was answered 404.
func TestAClientThatAddsItsOwnVersionIsGivenTheRootWithoutOne(t *testing.T) {
	t.Setenv("HAAS_MODEL_URL", "http://127.0.0.1:8041/model/v1")
	if got := ModelRoot(); got != "http://127.0.0.1:8041/model" {
		t.Errorf("the root reads as %q", got)
	}
	adapter, err := Load("", "claude")
	if err != nil {
		t.Fatal(err)
	}
	command := adapter.Command(context.Background(), job.Spec{Model: "anthropic/x"}, t.TempDir())
	var base string
	for _, set := range command.Env {
		if after, found := strings.CutPrefix(set, "ANTHROPIC_BASE_URL="); found {
			base = after
		}
	}
	if base != "http://127.0.0.1:8041/model" {
		t.Errorf("claude is pointed at %q, and it appends /v1/messages to that", base)
	}
}
