// Package harness runs the agent. A harness is a program, and an adapter is a
// file that says how to run it.
//
// Adding a harness is a JSON file: the program, its arguments, the environment
// it needs, and where the numbers sit in whatever it prints. No code and no
// release. The built-in adapters are defaults, and a directory named by
// HAAS_HARNESSES overrides any of them, so a customer can correct one or add a
// harness of their own without a new binary.
package harness

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

//go:embed adapters/*.json
var builtIn embed.FS

// Needs is what a harness needs before it can answer at all.
type Needs string

const (
	// NeedsNothing: a model in the building, and nothing outside it.
	NeedsNothing Needs = "nothing"
	// NeedsAccount: the user's own signed-in account, held by the CLI itself.
	NeedsAccount Needs = "account"
	// NeedsKey: an API key the proxy adds on the way out.
	NeedsKey Needs = "key"
)

// Stream is what the program prints.
type Stream string

const (
	// Plain text, for a person to read.
	Plain Stream = "plain"
	// JSONL: one JSON object a line, with the paths below saying where the
	// numbers are.
	JSONL Stream = "jsonl"
)

// Totals says whether a harness reports its usage per call or as a running
// total. Summing a running total counts every call more than once, which would
// make the spend figure a lie.
type Totals string

const (
	PerCall    Totals = "perCall"
	Cumulative Totals = "cumulative"
)

// Adapter is how to run one harness.
type Adapter struct {
	Name     string            `json:"name"`
	Program  string            `json:"program"`
	Argv     []string          `json:"argv"`
	Env      map[string]string `json:"env,omitempty"`
	Endpoint string            `json:"endpoint,omitempty"`
	Needs    Needs             `json:"needs"`
	Stream   Stream            `json:"stream"`
	Totals   Totals            `json:"totals,omitempty"`

	// Dotted paths into a JSONL line. An empty path means the harness does not
	// say, and the number is left out rather than guessed.
	Text         string `json:"text,omitempty"`
	InputTokens  string `json:"inputTokens,omitempty"`
	OutputTokens string `json:"outputTokens,omitempty"`
	CostUSD      string `json:"costUsd,omitempty"`
}

// Load the adapter a job names.
func Load(name string) (Adapter, error) {
	if dir := os.Getenv("HAAS_HARNESSES"); dir != "" {
		path := filepath.Join(dir, name+".json")
		if text, err := os.ReadFile(path); err == nil {
			return parse(text)
		}
	}
	text, err := builtIn.ReadFile("adapters/" + name + ".json")
	if err != nil {
		return Adapter{}, fmt.Errorf("no harness called %s", name)
	}
	return parse(text)
}

func parse(text []byte) (Adapter, error) {
	var adapter Adapter
	if err := json.Unmarshal(text, &adapter); err != nil {
		return Adapter{}, err
	}
	if adapter.Stream == "" {
		adapter.Stream = Plain
	}
	if adapter.Totals == "" {
		adapter.Totals = PerCall
	}
	return adapter, nil
}

// Names lists the adapters that ship with the binary.
func Names() []string {
	entries, _ := builtIn.ReadDir("adapters")
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, strings.TrimSuffix(entry.Name(), ".json"))
	}
	return names
}

// Command builds the process to run. The context carries the time budget, so a
// harness that will not stop is killed rather than waited for.
func (a Adapter) Command(ctx context.Context, spec job.Spec, workspace string) *exec.Cmd {
	argv := make([]string, 0, len(a.Argv))
	for _, argument := range a.Argv {
		argv = append(argv, a.fill(argument, spec))
	}
	command := exec.CommandContext(ctx, a.fill(a.Program, spec), argv...)
	command.Dir = workspace
	command.Env = os.Environ()
	for key, value := range a.Env {
		command.Env = append(command.Env, key+"="+a.fill(value, spec))
	}
	return command
}

func (a Adapter) fill(text string, spec job.Spec) string {
	return strings.NewReplacer(
		"{brief}", spec.Brief,
		"{model}", spec.Model,
		"{modelUrl}", ModelURL(),
	).Replace(text)
}

// ModelURL is where a local model answers. A container reaches the host's
// through host.docker.internal, so the runner is told rather than guessing.
func ModelURL() string {
	if url := os.Getenv("HAAS_MODEL_URL"); url != "" {
		return url
	}
	return "http://localhost:11434/v1"
}

// EndpointOf is the address a job will talk to, for the rules to check. A
// harness holding its own account names where that account lives.
func (a Adapter) EndpointOf() string {
	if a.Needs == NeedsNothing {
		return ModelURL()
	}
	return a.Endpoint
}

// Pick reads a dotted path out of a decoded JSONL line.
func Pick(line map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	var at any = line
	for _, key := range strings.Split(path, ".") {
		object, ok := at.(map[string]any)
		if !ok {
			return nil, false
		}
		at, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return at, true
}

// Number reads a dotted path as a number, which is how every token count and
// cost arrives over JSON.
func Number(line map[string]any, path string) (float64, bool) {
	value, ok := Pick(line, path)
	if !ok {
		return 0, false
	}
	number, ok := value.(float64)
	return number, ok
}
