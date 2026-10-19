package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/job"
)

func TestTheAdaptersThatShipParse(t *testing.T) {
	t.Setenv("GUSTO_HARNESSES", "")
	names := Names()
	if len(names) == 0 {
		t.Fatal("no adapter ships with this build")
	}
	for _, name := range names {
		adapter, err := Load("", name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if adapter.Name != name || adapter.Program == "" || (len(adapter.Argv) == 0 && adapter.Stream != ACP) {
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
	if !slices.Contains(command.Args, "--model=gusto-ollama/gpt-oss:20b") {
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
	t.Setenv("GUSTO_HARNESSES", mine)
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
	t.Setenv("GUSTO_HARNESSES", "")
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
	t.Setenv("GUSTO_HARNESSES", mine)
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

func TestAPathIntoAListThatIsNotThereIsNotFound(t *testing.T) {
	var said map[string]any
	if err := json.Unmarshal([]byte(`{"message":{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":2}}}`), &said); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"message.content.9.text", "message.content.x.text", "message.usage.0"} {
		if _, ok := Pick(said, path); ok {
			t.Errorf("%s should not resolve", path)
		}
	}
}

func TestWhatEachHarnessTalksTo(t *testing.T) {
	t.Setenv("GUSTO_MODEL_URL", "http://localhost:11434/v1")
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
	t.Setenv("GUSTO_HARNESSES", mine)
	names := Names()
	if !slices.Contains(names, "fixture") || !slices.Contains(names, "claude") {
		t.Fatalf("want both a harness of my own and the built-in ones, got %v", names)
	}
	if !slices.IsSorted(names) {
		t.Fatalf("a list a person reads should be sorted: %v", names)
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
	t.Setenv("GUSTO_MODEL_URL", "http://127.0.0.1:8041/model/v1")
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

// Every harness that says what it calls a conversation has to be able to
// continue one. Recording the session and never handing it back makes every
// message in a thread the first, which is how Codex answered "this is my
// first turn" on the sixth.
func TestEveryHarnessThatNamesItsSessionCanResumeIt(t *testing.T) {
	for _, name := range Names() {
		adapter, err := Load("", name)
		if err != nil {
			t.Fatal(err)
		}
		if adapter.SessionID == "" || adapter.Stream == ACP {
			continue
		}
		command := adapter.Command(context.Background(), job.Spec{Brief: "b", Remembered: "the-session"}, t.TempDir())
		if !strings.Contains(strings.Join(command.Args, " "), "the-session") {
			t.Errorf("%s records its session and never resumes it: %v", name, command.Args)
		}
	}
}

// pi reads its servers from a file rather than its arguments, and loads the
// extension that calls them only when there are servers to call.
func TestPiIsGivenItsServersInAFile(t *testing.T) {
	adapter, err := Load("", "pi")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "mcp", "mcp.json")
	adapter.MCP.File = file
	command := adapter.Command(context.Background(), job.Spec{Brief: "b"}, t.TempDir())
	if err := adapter.Give(command, []job.Server{{Name: "memory", Command: "mcp-server-memory"}}); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(written), `"mcpServers":{"memory"`) {
		t.Fatalf("the servers file holds %s, %v", written, err)
	}
	if !strings.Contains(strings.Join(command.Args, " "), "pi-mcp-adapter") {
		t.Errorf("pi was not given the adapter: %v", command.Args)
	}
}
