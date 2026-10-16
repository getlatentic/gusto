package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/signing"
)

// A record somebody kept the head of is checked with the flag before the file
// or after it, and lines cut off the end make the check fail.
func TestVerifyingARecordAgainstAHeadKeptEarlier(t *testing.T) {
	ring := t.TempDir()
	key, err := signing.Generate(ring, "acme")
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "changes.jsonl")
	log, err := changelog.Open(path, key, keyring)
	if err != nil {
		t.Fatal(err)
	}
	var head string
	for _, kind := range []string{changelog.SignedIn, changelog.SetupSaved, changelog.SkillWritten} {
		written, err := log.Append(changelog.Event{Who: "a@example.com", Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		head = written.Hash
	}
	log.Close()
	t.Setenv(signing.KeyringEnv, ring)

	if code := changesCommand([]string{"verify", path}); code != 0 {
		t.Fatalf("an intact record exited %d", code)
	}
	for name, args := range map[string][]string{
		"flag first": {"verify", "-head", head, path},
		"flag last":  {"verify", path, "-head", head},
	} {
		if code := changesCommand(args); code != 0 {
			t.Errorf("%s: the head is in the record and the check exited %d", name, code)
		}
	}

	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	if err := os.WriteFile(path, []byte(strings.Join(lines[:2], "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := changesCommand([]string{"verify", "-head", head, path}); code != 1 {
		t.Errorf("a record with its tail cut off exited %d, want 1", code)
	}
	if code := changesCommand([]string{"verify", path}); code != 0 {
		t.Errorf("without a head a shorter chain still holds, and exited %d", code)
	}

	lines[1] = strings.Replace(lines[1], "a@example.com", "b@example.com", 1)
	if err := os.WriteFile(path, []byte(strings.Join(lines[:2], "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := changesCommand([]string{"verify", path}); code != 1 {
		t.Errorf("an edited record exited %d, want 1", code)
	}
	if code := changesCommand([]string{"verify"}); code != 2 {
		t.Errorf("no file exited %d, want 2", code)
	}
}
