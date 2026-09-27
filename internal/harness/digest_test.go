package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureAdapter is the adapter the configured fixture harness runs.
const fixtureAdapter = `{"name":"fixture","program":"sh","argv":["-c","cat {harnessDir}/prompt.md"],"needs":"nothing"}` + "\n"

// fixtureDigest is the digest of the configured harness configuredFixture
// writes. It is written down rather than computed, so a change to the
// algorithm fails here instead of passing unnoticed, and it was checked
// against the shell steps in docs/FORMATS.md.
const fixtureDigest = "sha256:3e8c088aa809533e7f9fa7a54f545d896c2dcbe9a3c81b4d543ad97de277fa7f"

// configuredFixture writes a configured harness with a prompt, a skill, an
// executable helper and version control metadata beside it.
func configuredFixture(t *testing.T, root string) string {
	t.Helper()
	folder := filepath.Join(root, "fixture")
	put(t, folder, "adapter.json", fixtureAdapter, 0o644)
	put(t, folder, "prompt.md", "Answer yes.\n", 0o644)
	put(t, folder, "skills/yes/SKILL.md", "Say yes, and nothing else.\n", 0o644)
	put(t, folder, "bin/check", "#!/bin/sh\necho yes\n", 0o755)
	put(t, folder, ".git/HEAD", "ref: refs/heads/main\n", 0o644)
	return folder
}

func put(t *testing.T, folder, path, text string, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(folder, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(full, mode); err != nil {
		t.Fatal(err)
	}
}

func TestAConfiguredHarnessHasTheDigestWrittenDown(t *testing.T) {
	tree, err := walk(configuredFixture(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Digest(); got != fixtureDigest {
		t.Fatalf("digest %s, want %s", got, fixtureDigest)
	}
}

// What a checkout, a tarball and a copy each set afresh is not the harness.
func TestTheDigestIgnoresTimesOwnersAndReadBits(t *testing.T) {
	folder := configuredFixture(t, t.TempDir())
	prompt := filepath.Join(folder, "prompt.md")
	if err := os.Chmod(prompt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(prompt, time0, time0); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(folder, ".git")); err != nil {
		t.Fatal(err)
	}
	tree, err := walk(folder)
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Digest(); got != fixtureDigest {
		t.Fatalf("digest %s, want %s", got, fixtureDigest)
	}
}

func TestEveryPartOfAConfiguredHarnessMovesItsDigest(t *testing.T) {
	for _, change := range []func(folder string) error{
		func(folder string) error {
			return os.WriteFile(filepath.Join(folder, "prompt.md"), []byte("Answer no.\n"), 0o644)
		},
		func(folder string) error { return os.Chmod(filepath.Join(folder, "bin/check"), 0o644) },
		func(folder string) error {
			return os.Rename(filepath.Join(folder, "skills/yes"), filepath.Join(folder, "skills/aye"))
		},
		func(folder string) error { return os.WriteFile(filepath.Join(folder, "mcp.json"), []byte("{}"), 0o644) },
	} {
		folder := configuredFixture(t, t.TempDir())
		if err := change(folder); err != nil {
			t.Fatal(err)
		}
		tree, err := walk(folder)
		if err != nil {
			t.Fatal(err)
		}
		if tree.Digest() == fixtureDigest {
			t.Fatalf("a changed harness kept its digest: %v", tree)
		}
	}
}

func TestALinkInAConfiguredHarnessIsRefusedByName(t *testing.T) {
	folder := configuredFixture(t, t.TempDir())
	if err := os.Symlink("/etc/passwd", filepath.Join(folder, "skills/elsewhere")); err != nil {
		t.Fatal(err)
	}
	_, err := walk(folder)
	if err == nil || !strings.Contains(err.Error(), "skills/elsewhere") {
		t.Fatalf("want the link refused by name, got %v", err)
	}
}

// A bare adapter is the smallest configured harness, so it digests as a
// directory holding only that adapter does, whichever form it travels in.
func TestABareAdapterDigestsAsADirectoryHoldingOnlyIt(t *testing.T) {
	root := t.TempDir()
	put(t, root, "alone.json", fixtureAdapter, 0o644)
	put(t, root, "folded/adapter.json", fixtureAdapter, 0o644)
	alone, err := bare(filepath.Join(root, "alone.json"))
	if err != nil {
		t.Fatal(err)
	}
	folded, err := walk(filepath.Join(root, "folded"))
	if err != nil {
		t.Fatal(err)
	}
	if alone.Digest() != folded.Digest() {
		t.Fatalf("%s and %s", alone.Digest(), folded.Digest())
	}
}

var time0 = time.Unix(0, 0)
