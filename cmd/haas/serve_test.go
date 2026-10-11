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

// haas on a person's computer keeps its folders relative to its own, and a link's relative target is read from the link's
// folder, so the link has to name the workspace whole.
func TestATurnRunHereFindsTheWorkspaceWhenTheFoldersAreRelative(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("sessions", "a", "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := job.Dir(filepath.Join("jobs", "1"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := pointAt(filepath.Join("sessions", "a", "workspace"), dir); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir.Workspace()); err != nil || !info.IsDir() {
		t.Fatalf("the link goes nowhere: %v", err)
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
		proxy:  &proxy.Proxy{},
		tokens: proxy.NewTokens(),
		ledger: spend.NewLedger(spend.Policy{}),
	}
	held := proxy.Credentials{"platform": {Name: "platform", Address: "https://example.invalid"}}
	way.held.Store(&held)
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
