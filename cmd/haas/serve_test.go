package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/spend"
)

func TestASettingWrittenAsOneLineIsReadAsAList(t *testing.T) {
	t.Setenv("HAAS_MODELS", " platform/qwen.qwen3-vl-235b-a22b-instruct , ollama/llama3.2:3b ,, ")
	models := values("HAAS_MODELS")
	if len(models) != 2 || models[0] != "platform/qwen.qwen3-vl-235b-a22b-instruct" {
		t.Fatalf("got %v", models)
	}
}

func TestASettingNobodySetIsAnEmptyList(t *testing.T) {
	t.Setenv("HAAS_ALLOWED", "")
	if allowed := values("HAAS_ALLOWED"); len(allowed) != 0 {
		t.Fatalf("got %v", allowed)
	}
}

// A turn run in this process has no container to mount the instance into, so
// its workspace is a link. The job directory is made before it is queued, which
// leaves a real directory in the way — and a link cannot be put over one.
func TestATurnRunHereWorksInTheInstancesWorkspace(t *testing.T) {
	instance := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(instance, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance, "already.txt"), []byte("here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := pointAt(instance, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir.Workspace(), "already.txt")); err != nil {
		t.Fatalf("the turn does not work in the instance's workspace: %v", err)
	}
	// And doing it twice is doing it once, because a turn can be run again.
	if err := pointAt(instance, dir); err != nil {
		t.Fatalf("a second run could not point at it: %v", err)
	}
}

// A job with a checkout of its own is not a turn, and must not lose it.
func TestAWorkspaceWithSomethingInItIsNotReplacedByALink(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir.Workspace(), "mine.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pointAt(t.TempDir(), dir); err == nil {
		t.Fatal("a workspace with a file in it was replaced by a link")
	}
	if _, err := os.Stat(filepath.Join(dir.Workspace(), "mine.txt")); err != nil {
		t.Errorf("the file was lost: %v", err)
	}
}

// A job naming a credential this deployment does not hold is refused by name,
// before any container starts — not as a model call that fails somewhere inside
// a run somebody then has to go and read.
func TestAJobNamingACredentialNobodyHoldsIsRefusedBeforeItStarts(t *testing.T) {
	way := &WayOut{
		proxy:       &proxy.Proxy{},
		tokens:      proxy.NewTokens(),
		credentials: proxy.Credentials{"platform": {Name: "platform", Address: "https://example.invalid"}},
		ledger:      spend.NewLedger(spend.Policy{}),
	}
	dir := job.Dir(t.TempDir())
	_, err := way.Run(dir, job.Spec{Model: "nowhere/some-model"})
	if err == nil {
		t.Fatal("a credential nobody holds was allowed to start a container")
	}
	for _, want := range []string{"nowhere", "platform"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}

// Starting a harness from the catalog chooses no model, so it is given one. It
// must not be given one its credential will refuse: codex was handed the
// platform credential, whose provider rejects the tools codex sends, and the
// instance failed on its first message.
func TestAHarnessIsOnlyOfferedModelsItsCredentialWillServe(t *testing.T) {
	credentials := proxy.Credentials{
		// Measured: this one answers opencode and refuses what codex sends.
		"platform": {Name: "platform", Address: "https://one.invalid", Harnesses: []string{"opencode"}},
		"deepseek": {Name: "deepseek", Address: "https://two.invalid"},
	}
	offered := offeredModels(credentials,
		[]string{"platform/qwen", "deepseek/deepseek-flash"},
		[]string{"opencode", "codex"})

	if got := offered.First("codex"); got != "deepseek/deepseek-flash" {
		t.Errorf("codex would be started on %q", got)
	}
	if got := offered.For("codex"); len(got) != 1 {
		t.Errorf("codex is offered %v", got)
	}
	if got := offered.For("opencode"); len(got) != 2 {
		t.Errorf("opencode is offered %v, and its credential serves it", got)
	}
}

// A deployment that has found no exceptions says nothing, and every harness is
// offered everything.
func TestSayingNothingOffersEveryHarnessEverything(t *testing.T) {
	credentials := proxy.Credentials{"platform": {Name: "platform", Address: "https://one.invalid"}}
	offered := offeredModels(credentials, []string{"platform/qwen"}, []string{"opencode", "codex"})
	for _, harness := range []string{"opencode", "codex", "one-nobody-configured"} {
		if got := offered.First(harness); got != "platform/qwen" {
			t.Errorf("%s would be started on %q", harness, got)
		}
	}
}

// Claude Code speaks Anthropic's messages and every other harness here speaks
// OpenAI's. Offered a model through a credential in the other shape, it is
// told the model does not exist, which is how a Claude Code agent started from
// a template failed on its first message.
func TestAHarnessIsOnlyOfferedModelsInTheShapeItSpeaks(t *testing.T) {
	credentials := proxy.Credentials{
		"deepseek":  {Name: "deepseek", Address: "https://api.deepseek.invalid"},
		"anthropic": {Name: "anthropic", Address: "https://api.deepseek.invalid/anthropic", Auth: proxy.Anthropic},
	}
	offered := offeredModels(credentials,
		[]string{"deepseek/deepseek-flash", "anthropic/deepseek-flash"},
		[]string{"claude", "codex"})
	if got := offered.For("claude"); len(got) != 1 || got[0] != "anthropic/deepseek-flash" {
		t.Errorf("claude is offered %v", got)
	}
	if got := offered.For("codex"); len(got) != 1 || got[0] != "deepseek/deepseek-flash" {
		t.Errorf("codex is offered %v", got)
	}
}
