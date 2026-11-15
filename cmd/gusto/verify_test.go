package main

import (
	"testing"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/task"
)

// A message in a conversation names no check, so its record verifies on the
// signature and the files alone.
func TestAConversationMessageVerifiesWithoutACheck(t *testing.T) {
	dir := task.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := (task.Spec{Brief: "b", Harness: "h"}).Write(dir); err != nil {
		t.Fatal(err)
	}
	hash, files, err := record.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Write(dir, record.Manifest{Task: hash, Files: files}); err != nil {
		t.Fatal(err)
	}
	ring := t.TempDir()
	sealedBy(t, dir, ring, "greencloud")
	t.Setenv(signing.KeyringEnv, ring)
	if code := run([]string{"verify", dir.Path(), "-signed"}); code != 0 {
		t.Fatalf("verify -signed exited %d, want 0 for a signed message with no check", code)
	}
}
