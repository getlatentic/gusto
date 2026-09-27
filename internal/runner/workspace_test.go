package runner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/runner"
)

// A conversation works in one folder, turn after turn, and nothing commits
// between them. A turn's patch is what that turn changed, not everything the
// folder has gathered since it was made.
func TestATurnsPatchHoldsOnlyWhatThatTurnChanged(t *testing.T) {
	workspace := t.TempDir()
	if output, err := exec.Command("git", "-C", workspace, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	turn := func(file, text string) string {
		t.Helper()
		dir := job.Dir(t.TempDir())
		if err := dir.Make(); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(dir.Workspace()); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Symlink(workspace, dir.Workspace()); err != nil {
			t.Fatal(err)
		}
		before, err := runner.Snapshot(dir)
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(workspace, file), text)
		if err := runner.Diff(dir, before); err != nil {
			t.Fatal(err)
		}
		patch, err := os.ReadFile(dir.OutFile("diff.patch"))
		if err != nil {
			t.Fatal(err)
		}
		return string(patch)
	}
	first := turn("notes.md", "the secret word is heron\n")
	second := turn("other.md", "something else\n")
	if !strings.Contains(first, "notes.md") {
		t.Fatalf("the first turn's patch lacks its own change:\n%s", first)
	}
	if strings.Contains(second, "notes.md") || !strings.Contains(second, "other.md") {
		t.Fatalf("the second turn's patch should hold other.md alone:\n%s", second)
	}
	status, _ := exec.Command("git", "-C", workspace, "status", "--porcelain").Output()
	if !strings.Contains(string(status), "?? notes.md") {
		t.Errorf("the workspace's own index was changed: %q", status)
	}
}
