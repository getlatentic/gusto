package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/haas/internal/job"
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
