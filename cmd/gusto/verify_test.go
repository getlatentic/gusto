package main

import (
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// A message in a conversation names no gate, so its record verifies on the
// signature and the files alone.
func TestAConversationMessageVerifiesWithoutAGate(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := (job.Spec{Brief: "b", Harness: "h"}).Write(dir); err != nil {
		t.Fatal(err)
	}
	hash, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Job: hash, Files: files}); err != nil {
		t.Fatal(err)
	}
	ring := t.TempDir()
	sealedBy(t, dir, ring, "greencloud")
	t.Setenv(signing.KeyringEnv, ring)
	if code := run([]string{"verify", dir.Path(), "-signed"}); code != 0 {
		t.Fatalf("verify -signed exited %d, want 0 for a signed message with no gate", code)
	}
}
