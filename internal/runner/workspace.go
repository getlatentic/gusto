package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/getlatentic/haas/internal/job"
)

// Fill puts the repository in workspace/. A directory that already holds a
// checkout is left alone, so a run can be repeated against the same tree.
func Fill(spec job.Spec, dir job.Dir) error {
	if _, err := os.Stat(filepath.Join(dir.Workspace(), ".git")); err == nil {
		return nil
	}
	if err := git(dir.Path(), "clone", "--quiet", spec.Repo, "workspace"); err != nil {
		return err
	}
	return git(dir.Workspace(), "checkout", "--quiet", spec.Ref)
}

// Diff writes what the harness changed. `add -N` puts new files in the index as
// intentions, so the patch shows them without staging their contents.
func Diff(dir job.Dir) error {
	if err := git(dir.Workspace(), "add", "-N", "."); err != nil {
		return err
	}
	patch, err := exec.Command("git", "-C", dir.Workspace(), "diff").Output()
	if err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile("diff.patch"), patch, 0o644)
}

func git(at string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = at
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, output)
	}
	return nil
}
