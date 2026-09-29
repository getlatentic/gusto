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

// The runner is root in the box, and a workspace's git configuration belongs to
// the agent. Staging the workspace runs nothing the workspace's own .git names.
func TestStagingAWorkspaceRunsNothingItsOwnGitConfigurationNames(t *testing.T) {
	// Other packages' tests stage workspaces in the shared temp directory while
	// this one runs, so what is left behind is only looked for in a directory of its own.
	t.Setenv("TMPDIR", t.TempDir())
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	workspace := dir.Workspace()
	run := func(args ...string) {
		t.Helper()
		if output, err := exec.Command("git", append([]string{"-C", workspace}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	run("init", "--quiet")
	marker := filepath.Join(t.TempDir(), "ran")
	write(t, filepath.Join(workspace, ".gitattributes"), "* filter=x diff=x\n")
	write(t, filepath.Join(workspace, "code.txt"), "one\n")
	run("config", "filter.x.clean", "sh -c 'touch "+marker+"; cat'")
	run("config", "diff.x.textconv", "sh -c 'touch "+marker+"; cat'")
	run("config", "core.fsmonitor", "sh -c 'touch "+marker+"; exit 1'")
	run("config", "core.hooksPath", "/nonexistent")

	before, err := runner.Snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(workspace, "code.txt"), "two\n")
	if err := runner.Diff(dir, before); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the runner ran a program the workspace's git configuration named")
	}
	patch, _ := os.ReadFile(dir.OutFile("diff.patch"))
	if !strings.Contains(string(patch), "+two") || !strings.Contains(string(patch), "-one") {
		t.Errorf("the patch does not say what changed:\n%s", patch)
	}
	if entries, _ := os.ReadDir(os.TempDir()); len(entries) > 0 {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "haas-trees-") {
				if _, err := os.Stat(filepath.Join(os.TempDir(), entry.Name(), "HEAD")); err == nil {
					t.Errorf("%s was left behind", entry.Name())
				}
			}
		}
	}
}
