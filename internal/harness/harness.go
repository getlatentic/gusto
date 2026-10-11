// Package harness runs the agent. A harness is a program, and an adapter is a
// file that says how to run it.
//
// Adding a harness is a JSON file: the program, its arguments, the environment
// it needs, and where the numbers sit in whatever it prints. No code and no
// release. The built-in adapters are defaults, and a directory named by
// HAAS_HARNESSES overrides any of them, so a customer can correct one or add a
// harness of their own without a new binary.
//
// A configured harness is the adapter and what shapes the harness it runs: a
// prompt, skills, MCP servers, a model policy. It is a directory with the
// adapter at adapter.json, and a bare name.json is the smallest one, an adapter
// alone. Its digest names it by what it holds rather than by what it is called.
package harness

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/wire"
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
	// Title is what a person who has never heard of the program calls it.
	Title string `json:"title,omitempty"`
	// What this harness is, in one line, for somebody choosing between them.
	// A catalog that lists only names is not a choice.
	What    string `json:"what,omitempty"`
	Program string `json:"program"`
	// Package is the npm package that provides the program, as the runner image installs it, and as a person installs it on
	// their own computer.
	Package string `json:"package,omitempty"`
	// Installs is the program the package puts on the PATH, when the adapter starts it through another: hermes is started by
	// sh. It is what a catalog looks for. Empty is the program itself.
	Installs string            `json:"installs,omitempty"`
	Argv     []string          `json:"argv"`
	Env      map[string]string `json:"env,omitempty"`
	Endpoint string            `json:"endpoint,omitempty"`
	// Stdin is what the harness reads on standard input. Some take the brief as
	// an argument and some read it: codex waits on stdin forever otherwise.
	Stdin string `json:"stdin,omitempty"`
	// Skills is the folder the harness finds skills in, each a directory with
	// a SKILL.md. The runner puts a job's skills there before it starts. Empty
	// is a harness that takes none.
	Skills string `json:"skills,omitempty"`
	// Speaks is the shape of API the harness calls a model in: "anthropic"
	// for Anthropic's Messages, "responses" for OpenAI's Responses, and Chat
	// Completions when it is left out. A model is only offered to a harness
	// through a credential that answers in its shape or translates it.
	Speaks string `json:"speaks,omitempty"`
	// Keeps is the folder the harness keeps its own state in: its settings and
	// the conversations it resumes. It has to be under the instance's state,
	// which is the one folder that outlives a turn's box, or every next message
	// looks for a conversation that is gone. The runner makes it, because some
	// harnesses refuse to start without it.
	Keeps string `json:"keeps,omitempty"`
	// WhenBoxed swaps an argument for another when the box the harness runs in
	// confines it already. A harness that makes its own sandbox out of namespaces
	// cannot make one inside a sandbox that forbids them, and there the box is
	// the boundary.
	WhenBoxed map[string]string `json:"whenBoxed,omitempty"`
	// MCP is how the harness takes MCP servers, or nil when it takes none.
	MCP    *MCP   `json:"mcp,omitempty"`
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

	// home is the directory of the configured harness this adapter came from,
	// which {harnessDir} names so the harness can read its prompt, skills and
	// servers. It is empty for an adapter with no directory of its own.
	home string
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

// Command builds the process to run. The context carries the time budget, so a
// harness that will not stop is killed rather than waited for.
// Shape is the API the harness calls a model in.
func (a Adapter) Shape() wire.Shape {
	switch a.Speaks {
	case "anthropic":
		return wire.Messages
	case "responses":
		return wire.Responses
	}
	return wire.Chat
}

func (a Adapter) Command(ctx context.Context, spec job.Spec, workspace string) *exec.Cmd {
	return a.CommandIn(ctx, spec, workspace, Here())
}

// Installed is the program a computer or an image needs for this harness to run.
func (a Adapter) Installed() string {
	if a.Installs != "" {
		return a.Installs
	}
	return a.Program
}

// Env is what a harness is started with besides its own arguments: the
// environment it inherits, and where its model is and what stands in for a
// credential. A box that is not this process names them itself, so nothing of
// this process's environment, which holds the server's own secrets, reaches it.
type Env struct {
	// Boxed says the box the harness starts in confines it, so an adapter's
	// WhenBoxed applies.
	Boxed      bool
	Base       []string
	ModelURL   string
	ModelToken string
	// Home and Data are the harness's home and the folder it keeps its state in when it runs outside a container, where
	// they stand for /root and /root/.local/share. Empty is a harness in the container its adapter was written for.
	Home, Data string
}

// Here is this process's own: what a container's runner and a run on a laptop
// start a harness with.
func Here() Env {
	return Env{Base: os.Environ(), ModelURL: ModelURL(), ModelToken: ModelToken()}
}

// CommandIn is Command for a harness started in an environment given.
func (a Adapter) CommandIn(ctx context.Context, spec job.Spec, workspace string, env Env) *exec.Cmd {
	wanted := a.Argv
	if spec.Remembered != "" && len(a.Resume) > 0 {
		wanted = a.Resume
	}
	argv := make([]string, 0, len(wanted))
	for _, argument := range wanted {
		// An argument the job left empty is dropped rather than passed as a
		// bare flag: a harness asked for "--model=" refuses to start, and the
		// job meant "whatever you are configured with".
		filled := a.fill(argument, spec, env)
		if instead, swapped := a.WhenBoxed[filled]; swapped && env.Boxed {
			filled = instead
		}
		if filled == "" || strings.HasSuffix(filled, "=") {
			continue
		}
		argv = append(argv, filled)
	}
	command := exec.CommandContext(ctx, a.fill(a.Program, spec, env), argv...)
	command.Dir = workspace
	// A folder a job names is looked at again here, and one that leaves the workspace is not gone to.
	if dir, err := job.CleanDir(spec.Dir); err == nil && dir != "" {
		command.Dir = filepath.Join(workspace, filepath.FromSlash(dir))
	}
	if a.Stdin != "" {
		command.Stdin = strings.NewReader(a.fill(a.Stdin, spec, env))
	}
	command.Env = append([]string{}, env.Base...)
	for key, value := range a.Env {
		command.Env = append(command.Env, key+"="+a.fill(value, spec, env))
	}
	return command
}

func (a Adapter) fill(text string, spec job.Spec, env Env) string {
	provider, alone, found := strings.Cut(spec.Model, "/")
	if !found {
		provider, alone = "", spec.Model
	}
	// A harness that runs only the models its configuration names is told of the job's own, under the name of the
	// credential that serves it, and of none when the job names no model: a provider called haas, which its
	// configuration turns off.
	if provider == "" {
		provider = "haas"
	}
	return strings.NewReplacer(
		"{brief}", spec.Brief,
		// A job names its model as the credential that serves it and then the
		// model: `platform/qwen…`. A harness with its own provider list wants
		// the whole thing, and one that passes the name straight to a provider
		// wants the model alone.
		"{model}", spec.Model,
		"{modelId}", alone,
		"{modelProvider}", provider,
		"{modelUrl}", env.ModelURL,
		"{modelRoot}", strings.TrimSuffix(env.ModelURL, "/v1"),
		"{modelToken}", env.ModelToken,
		"{session}", spec.Remembered,
		"{harnessDir}", a.home,
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
