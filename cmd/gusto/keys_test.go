package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/signing"
)

// sealedJob leaves a finished, signed job under root.
func sealedJob(t *testing.T, root, id string, signer signing.Signer) {
	t.Helper()
	dir := job.Dir(filepath.Join(root, id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir.Spec(), []byte(`{"brief": "b", "repo": "r", "gate": ["true"], "harness": "h"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir.OutFile("answer.txt"), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Job: spec, Files: files, Signer: "acme"}); err != nil {
		t.Fatal(err)
	}
	if err := evidence.Seal(dir, signer); err != nil {
		t.Fatal(err)
	}
}

// The exit code is what a script that retires a key reads: nonzero while any
// signed record has no key in the keyring to name it.
func TestKeysCheckFailsWhileARecordsKeyIsNotInTheKeyring(t *testing.T) {
	ring, root := t.TempDir(), t.TempDir()
	key, err := signing.Generate(ring, "acme")
	if err != nil {
		t.Fatal(err)
	}
	sealedJob(t, root, "a", key)
	t.Setenv(signing.KeyringEnv, ring)

	if code := checkKeys([]string{"-tasks", root}); code != 0 {
		t.Fatalf("with every key held the check exited %d", code)
	}
	if err := os.Remove(filepath.Join(ring, "acme.pub")); err != nil {
		t.Fatal(err)
	}
	if code := checkKeys([]string{"-tasks", root}); code != 1 {
		t.Fatalf("with the key gone the check exited %d, want 1", code)
	}
}
