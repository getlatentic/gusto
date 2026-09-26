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
	if len(names) != 3 {
		t.Fatalf("want three built-in adapters, got %v", names)
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
