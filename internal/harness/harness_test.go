package harness

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

func TestTheAdaptersThatShipParse(t *testing.T) {
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
