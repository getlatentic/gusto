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
	"sort"
	"strconv"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

//go:embed adapters/*.json
var builtIn embed.FS

// Needs is what a harness needs before it can answer at all.
type Needs string

const (
	// NeedsNothing: no model at all. A harness can be a program that makes the
	// change, and nothing about it leaves the machine.
	NeedsNothing Needs = "nothing"
	// NeedsLocal: a model in the building, and nothing outside it.
	NeedsLocal Needs = "local"
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
	// ACP: the harness speaks the Agent Client Protocol, and the runner holds
	// the conversation rather than reading a printout. The protocol fixes where
	// everything is said, so the paths default to it.
	ACP Stream = "acp"
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
	Name string `json:"name"`
	// What this harness is, in one line, for somebody choosing between them.
	// A catalog that lists only names is not a choice.
	What     string            `json:"what,omitempty"`
	Program  string            `json:"program"`
	Argv     []string          `json:"argv"`
	Env      map[string]string `json:"env,omitempty"`
	Endpoint string            `json:"endpoint,omitempty"`
	// Stdin is what the harness reads on standard input. Some take the brief as
	// an argument and some read it: codex waits on stdin forever otherwise.
	Stdin  string `json:"stdin,omitempty"`
	Needs  Needs  `json:"needs"`
	Stream Stream `json:"stream"`
	Totals Totals `json:"totals,omitempty"`

	// Dotted paths into a JSONL line. An empty path means the harness does not
	// say, and the number is left out rather than guessed.
	// Resume replaces the arguments when there is a conversation to continue. It
	// is the whole list rather than a fragment: splicing two lists duplicates
	// whatever flag they share.
	Resume []string `json:"resume,omitempty"`

	// When narrows which lines each of the readings below is taken from, by
	// purpose: "text", "usage" and "tool". A harness that says a thing once
	// needs none of this. pi says everything three times — one message's usage
	// rides on message_update, message_end and turn_end alike, its thinking and
	// its tool arguments stream on the same path as its words, and a tool call
	// appears at its start, its update and its end. Reading every line that
	// carries a thing counted a run at nearly three times what it spent, put
	// the model's reasoning in the transcript, and wrote each command down
	// three times.
	When map[string]map[string]string `json:"when,omitempty"`

	Text string `json:"text,omitempty"`
	// Message is where a harness names the message a piece of text belongs to.
	// Text from a new message starts a new paragraph, so two replies do not
	// run together as one sentence.
	Message string `json:"message,omitempty"`
	// Where a harness says what it ran. A line on which ToolName resolves is a
	// tool call, and nothing else distinguishes one.
	ToolName   string `json:"toolName,omitempty"`
	ToolInput  string `json:"toolInput,omitempty"`
	ToolStatus string `json:"toolStatus,omitempty"`
	ToolTitle  string `json:"toolTitle,omitempty"`
	// SessionID is where the harness says what it calls this conversation.
	SessionID    string `json:"sessionId,omitempty"`
	InputTokens  string `json:"inputTokens,omitempty"`
	OutputTokens string `json:"outputTokens,omitempty"`
	// Cache tokens are billed, so a total that leaves them out disagrees with
	// the cost beside it. Claude Code's preamble is almost all cache writes.
	CacheWriteTokens string `json:"cacheWriteTokens,omitempty"`
	CacheReadTokens  string `json:"cacheReadTokens,omitempty"`
	// TotalTokens is where a harness states its own total. It is taken over
	// the parts, whose overlaps differ from one harness to the next.
	TotalTokens string `json:"totalTokens,omitempty"`
	CostUSD     string `json:"costUsd,omitempty"`
}

// Load the adapter a job names, looking in the job's own directory first.
//
// An adapter in the job travels with it, so a job naming a harness of your own
// runs on a runner that has never heard of it — in a container, on a fleet, on
// somebody else's machine. That is the same reason everything else about a run
// lives in the directory.
func Load(dir job.Dir, name string) (Adapter, error) {
	for _, root := range roots(dir) {
		if text, err := os.ReadFile(filepath.Join(root, name+".json")); err == nil {
			return parse(text)
		}
	}
	text, err := builtIn.ReadFile("adapters/" + name + ".json")
	if err != nil {
		return Adapter{}, fmt.Errorf("no harness called %s", name)
	}
	return parse(text)
}

// Carried is where an adapter sits inside a job directory.
func Carried(dir job.Dir) string {
	return filepath.Join(dir.Path(), "harnesses")
}

func roots(dir job.Dir) []string {
	var roots []string
	if dir != "" {
		roots = append(roots, Carried(dir))
	}
	if mine := os.Getenv("HAAS_HARNESSES"); mine != "" {
		roots = append(roots, mine)
	}
	return roots
}

// Keep writes an adapter into the job, so that whoever runs the job next has it.
// A built-in adapter is already everywhere, so only one of your own is copied.
func Keep(dir job.Dir, name string) error {
	if _, err := builtIn.ReadFile("adapters/" + name + ".json"); err == nil {
		return nil
	}
	mine := os.Getenv("HAAS_HARNESSES")
	if mine == "" {
		return nil
	}
	text, err := os.ReadFile(filepath.Join(mine, name+".json"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(Carried(dir), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(Carried(dir), name+".json"), text, 0o644)
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
	if adapter.Stream == ACP {
		adapter.speaksACP()
	}
	return adapter, nil
}

// Names lists every harness this binary can run: the adapters it ships with,
// and any of your own in HAAS_HARNESSES. The pages offer what the server can
// actually run, rather than what it was compiled with.
func Names() []string {
	found := map[string]bool{}
	entries, _ := builtIn.ReadDir("adapters")
	for _, entry := range entries {
		found[strings.TrimSuffix(entry.Name(), ".json")] = true
	}
	if mine := os.Getenv("HAAS_HARNESSES"); mine != "" {
		yours, _ := os.ReadDir(mine)
		for _, entry := range yours {
			if strings.HasSuffix(entry.Name(), ".json") {
				found[strings.TrimSuffix(entry.Name(), ".json")] = true
			}
		}
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Command builds the process to run. The context carries the time budget, so a
// harness that will not stop is killed rather than waited for.
func (a Adapter) Command(ctx context.Context, spec job.Spec, workspace string) *exec.Cmd {
	wanted := a.Argv
	if spec.Remembered != "" && len(a.Resume) > 0 {
		wanted = a.Resume
	}
	argv := make([]string, 0, len(wanted))
	for _, argument := range wanted {
		// An argument the job left empty is dropped rather than passed as a
		// bare flag: a harness asked for "--model=" refuses to start, and the
		// job meant "whatever you are configured with".
		filled := a.fill(argument, spec)
		if filled == "" || strings.HasSuffix(filled, "=") {
			continue
		}
		argv = append(argv, filled)
	}
	command := exec.CommandContext(ctx, a.fill(a.Program, spec), argv...)
	command.Dir = workspace
	if a.Stdin != "" {
		command.Stdin = strings.NewReader(a.fill(a.Stdin, spec))
	}
	command.Env = os.Environ()
	for key, value := range a.Env {
		command.Env = append(command.Env, key+"="+a.fill(value, spec))
	}
	return command
}

func (a Adapter) fill(text string, spec job.Spec) string {
	_, alone, _ := strings.Cut(spec.Model, "/")
	if alone == "" {
		alone = spec.Model
	}
	return strings.NewReplacer(
		"{brief}", spec.Brief,
		// A job names its model as the credential that serves it and then the
		// model: `platform/qwen…`. A harness with its own provider list wants
		// the whole thing, and one that passes the name straight to a provider
		// wants the model alone.
		"{model}", spec.Model,
		"{modelId}", alone,
		"{modelUrl}", ModelURL(),
		"{modelRoot}", ModelRoot(),
		"{modelToken}", ModelToken(),
		"{session}", spec.Remembered,
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

// ModelRoot is where the model answers, without the version on the end, for a
// client that adds its own. An Anthropic-shaped one appends /v1/messages, so
// handing it the /v1 form asks the provider for /v1/v1/messages and is answered
// 404.
func ModelRoot() string { return strings.TrimSuffix(ModelURL(), "/v1") }

// ModelToken is what the box was given in place of a credential. A harness that
// reads its key from a variable of its own is handed this, so the real key
// stays in the proxy and this is worth nothing once the job ends.
func ModelToken() string { return os.Getenv("HAAS_MODEL_TOKEN") }

// EndpointOf is the address a job will talk to, for the rules to check.
//
// A harness that needs no model talks to nothing, one needing a model in the
// building talks to it, and one taking a key names the provider its credential
// reaches.
//
// It is deliberately not the proxy's address for a harness taking a key. The
// box is handed the proxy, but the proxy forwards to a provider outside the
// building, and a rule that asked the box where it was told to call would read
// a restricted job as staying in the building when it does not. The address
// that would answer this exactly is the credential's, which the deployment
// holds and the adapter cannot see.
func (a Adapter) EndpointOf() string {
	switch a.Needs {
	case NeedsNothing:
		return ""
	case NeedsLocal:
		return ModelURL()
	default:
		return a.Endpoint
	}
}

// Pick reads a dotted path out of a decoded JSONL line.
func Pick(line map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	var at any = line
	for _, key := range strings.Split(path, ".") {
		// A number addresses a list, because a harness that reports its text as
		// a list of blocks is normal: Claude Code says message.content.0.text.
		if index, err := strconv.Atoi(key); err == nil {
			list, ok := at.([]any)
			if !ok || index < 0 || index >= len(list) {
				return nil, false
			}
			at = list[index]
			continue
		}
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

// Says, Counts and Ran report whether this line is the one the reply, the
// numbers and a tool call are read from. An adapter that narrows nothing reads
// each from every line that carries it.
func (a Adapter) Says(line map[string]any) bool   { return a.reads("text", line) }
func (a Adapter) Counts(line map[string]any) bool { return a.reads("usage", line) }
func (a Adapter) Ran(line map[string]any) bool    { return a.reads("tool", line) }

func (a Adapter) reads(purpose string, line map[string]any) bool {
	for path, want := range a.When[purpose] {
		got, found := Pick(line, path)
		if !found || fmt.Sprint(got) != want {
			return false
		}
	}
	return true
}
