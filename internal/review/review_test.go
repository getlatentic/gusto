package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlyTheNamesAJobDirectoryHoldsAreUnpacked(t *testing.T) {
	for name, want := range map[string]bool{
		"job.json":              true,
		"out/manifest.json":     true,
		"out/transcript.log":    true,
		"workspace/answer.txt":  false,
		"out/nested/file":       false,
		"out/..":                false,
		"out/":                  false,
		".git/hooks/pre-commit": false,
		"answer.txt":            false,
	} {
		if got := jobFile(name); got != want {
			t.Errorf("%s: got %t, want %t", name, got, want)
		}
	}
	for mode, want := range map[string]bool{"100644": true, "100755": true, "120000": false, "160000": false, "040000": false} {
		if got := plainFile(mode); got != want {
			t.Errorf("mode %s: got %t, want %t", mode, got, want)
		}
	}
}

func TestADigestThatIsNotASha256IsRefusedBeforeAnythingIsFetched(t *testing.T) {
	for _, digest := range []string{"", "abc", strings.Repeat("A", 64), strings.Repeat("a", 63) + "/"} {
		if _, err := Check(context.Background(), "/nowhere", digest, filepath.Join(t.TempDir(), "job")); err == nil {
			t.Errorf("%q should be refused", digest)
		}
	}
}

func TestADirectoryThatHoldsSomethingIsNotUnpackedInto(t *testing.T) {
	into := t.TempDir()
	if err := os.WriteFile(filepath.Join(into, "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Check(context.Background(), "/nowhere", strings.Repeat("a", 64), into)
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("want a refusal naming the directory, got %v", err)
	}
}

func TestARemoteWithNoEvidenceSaysSo(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	bare := filepath.Join(t.TempDir(), "remote.git")
	if output, err := exec.Command("git", "init", "--quiet", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	_, err := Check(context.Background(), bare, strings.Repeat("a", 64), filepath.Join(t.TempDir(), "job"))
	if err == nil || !strings.Contains(err.Error(), "holds no record for that manifest") {
		t.Fatalf("want a missing ref named, got %v", err)
	}
}
