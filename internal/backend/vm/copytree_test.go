package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestATreeIsCopiedWithItsLinksAsLinksAndNeverFollowedOutOfIt(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("not the agent's\n"), 0o600)
	from := filepath.Join(t.TempDir(), "workspace")
	os.MkdirAll(filepath.Join(from, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(from, "a.txt"), []byte("a\n"), 0o644)
	os.WriteFile(filepath.Join(from, "sub", "deep", "run.sh"), []byte("#!/bin/sh\n"), 0o755)
	os.Symlink(outside, filepath.Join(from, "link-to-secret"))
	os.Symlink("a.txt", filepath.Join(from, "link-to-a"))
	to := filepath.Join(t.TempDir(), "copy")
	os.MkdirAll(to, 0o755)
	os.WriteFile(filepath.Join(to, "stale.txt"), []byte("old\n"), 0o644)

	if err := replaceTree(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "stale.txt")); err == nil {
		t.Error("what was there is replaced and not merged")
	}
	if info, err := os.Lstat(filepath.Join(to, "link-to-secret")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("a link is a link, and its target is not copied in: %v %v", info, err)
	}
	if target, _ := os.Readlink(filepath.Join(to, "link-to-a")); target != "a.txt" {
		t.Errorf("%q", target)
	}
	if info, err := os.Stat(filepath.Join(to, "sub", "deep", "run.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("a file keeps its mode: %v %v", info, err)
	}
	if got, _ := os.ReadFile(filepath.Join(to, "a.txt")); string(got) != "a\n" {
		t.Errorf("%q", got)
	}
	for _, leftover := range []string{to + ".next", to + ".old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s was left behind", leftover)
		}
	}
}

func TestACopyThatFailsLeavesWhatWasThere(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads what a mode of 0 forbids")
	}
	from := filepath.Join(t.TempDir(), "workspace")
	os.MkdirAll(from, 0o755)
	os.WriteFile(filepath.Join(from, "fine.txt"), []byte("fine\n"), 0o644)
	os.WriteFile(filepath.Join(from, "unreadable.txt"), []byte("no\n"), 0o000)
	to := filepath.Join(t.TempDir(), "copy")
	os.MkdirAll(to, 0o755)
	os.WriteFile(filepath.Join(to, "kept.txt"), []byte("kept\n"), 0o644)
	if err := replaceTree(from, to); err == nil {
		t.Fatal("a file that cannot be read fails the copy")
	}
	if got, _ := os.ReadFile(filepath.Join(to, "kept.txt")); string(got) != "kept\n" {
		t.Errorf("what was there is left: %q", got)
	}
	if _, err := os.Stat(to + ".next"); err == nil {
		t.Error("a half-made copy was left behind")
	}
}
