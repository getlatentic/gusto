package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/repoaccess"
)

// Fill puts the repository in workspace/, or makes an empty one when no
// repository is named. The clone happens here, outside the box, so a credential
// for the repository never goes in with the agent. A directory that already holds a checkout is left alone,
// so a run can be repeated against the same tree.
func Fill(spec job.Spec, dir job.Dir) error {
	// A turn in a conversation works in the conversation's directory, which is
	// mounted over this one. Cloning here would write files nobody reads.
	if spec.Session != "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir.Workspace(), ".git")); err == nil {
		return nil
	}
	if spec.Repo == "" {
		// Not every task starts from a repository. The workspace is one anyway,
		// because a run says what it changed by diffing it.
		return git(dir.Workspace(), "init", "--quiet", ".")
	}
	forge, err := repoaccess.FromEnv()
	if err != nil {
		return err
	}
	if err := forge.Clone(context.Background(), spec.Repo, dir.Path(), "workspace"); err != nil {
		// A repository the App does not reach is a refusal, said in the
		// record like any other, and not a run that failed somewhere.
		if errors.As(err, &repoaccess.NotInstalledError{}) {
			return Refuse(dir, err)
		}
		return err
	}
	return git(dir.Workspace(), "checkout", "--quiet", spec.Ref)
}

// Snapshot is the workspace as a git tree, before the harness touches it. A
// turn in a conversation works in a folder earlier turns already changed, and
// what this turn changed is the difference from here, not from the folder's
// first commit.
func Snapshot(dir job.Dir) (string, error) {
	return treeOf(dir.Workspace())
}

// Diff writes what the harness changed since before: every file added,
// removed or edited, new ones with their contents.
func Diff(dir job.Dir, before string) error {
	after, err := treeOf(dir.Workspace())
	if err != nil {
		return err
	}
	patch, err := exec.Command("git", "-C", dir.Workspace(), "diff", before, after).Output()
	if err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile("diff.patch"), patch, 0o644)
}

// treeOf writes the working tree, ignored files left out, as a tree object.
// It stages into an index of its own, so the workspace's index and history
// are exactly as the harness left them.
func treeOf(workspace string) (string, error) {
	scratch, err := os.MkdirTemp("", "haas-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(scratch)
	index := "GIT_INDEX_FILE=" + filepath.Join(scratch, "index")
	if err := gitWith(workspace, index, "add", "-A", "."); err != nil {
		return "", err
	}
	command := exec.Command("git", "write-tree")
	command.Dir = workspace
	command.Env = append(os.Environ(), index)
	tree, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git write-tree: %w", err)
	}
	return strings.TrimSpace(string(tree)), nil
}

func git(at string, args ...string) error {
	return gitWith(at, "", args...)
}

// gitWith runs git with one more variable in its environment.
func gitWith(at, variable string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = at
	if variable != "" {
		command.Env = append(os.Environ(), variable)
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, output)
	}
	return nil
}
