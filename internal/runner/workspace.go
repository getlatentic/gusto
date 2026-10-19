package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/repoaccess"
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
	if err := repoaccess.AcceptableRef(spec.Ref); err != nil {
		return Refuse(dir, err)
	}
	return git(dir.Workspace(), "checkout", "--quiet", spec.Ref)
}

// Snapshot is the workspace as a git tree, before the agent touches it. A
// turn in a conversation works in a folder earlier turns already changed, and
// what this turn changed is the difference from here, not from the folder's
// first commit.
//
// The trees are made in a repository of the runner's own, with the workspace as
// its working tree. A workspace's own .git is the agent's to write, and git runs
// the programs its configuration names (a filter, an fsmonitor) when it stages
// files; the runner is root, so staging with the agent's configuration would
// hand the agent root in the box during the one step that writes the record.
func Snapshot(dir job.Dir) (string, error) {
	return treeOf(dir)
}

// Diff writes what the agent changed since before: every file added,
// removed or edited, new ones with their contents.
func Diff(dir job.Dir, before string) error {
	defer os.RemoveAll(objectsOf(dir))
	after, err := treeOf(dir)
	if err != nil {
		return err
	}
	command := exec.Command("git", "diff", before, after)
	command.Env = ownRepository(dir)
	patch, err := command.Output()
	if err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile("diff.patch"), patch, 0o644)
}

// objectsOf is where a job's trees are kept while it runs, outside the job
// directory and the workspace, so neither the agent nor the evidence holds it.
func objectsOf(dir job.Dir) string {
	path, err := filepath.Abs(dir.Path())
	if err != nil {
		path = dir.Path()
	}
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(os.TempDir(), "gusto-trees-"+hex.EncodeToString(sum[:8]))
}

// ownRepository is the environment that makes git use the runner's repository
// and the job's workspace, and nothing else's configuration: no system or user
// file, and no prompt.
func ownRepository(dir job.Dir) []string {
	workspace, err := filepath.Abs(dir.Workspace())
	if err != nil {
		workspace = dir.Workspace()
	}
	return append(os.Environ(),
		"GIT_DIR="+objectsOf(dir),
		"GIT_WORK_TREE="+workspace,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_INDEX_FILE="+filepath.Join(objectsOf(dir), "index"),
		"GIT_TERMINAL_PROMPT=0",
	)
}

// treeOf writes the working tree, ignored files left out, as a tree object.
func treeOf(dir job.Dir) (string, error) {
	if _, err := os.Stat(filepath.Join(objectsOf(dir), "HEAD")); err != nil {
		if output, err := exec.Command("git", "init", "--quiet", "--bare", objectsOf(dir)).CombinedOutput(); err != nil {
			return "", fmt.Errorf("git init: %w: %s", err, output)
		}
	}
	for _, args := range [][]string{{"add", "-A", "."}, {"write-tree"}} {
		command := exec.Command("git", args...)
		command.Dir = dir.Workspace()
		command.Env = ownRepository(dir)
		output, err := command.Output()
		if err != nil {
			return "", fmt.Errorf("git %s: %w", args[0], err)
		}
		if args[0] == "write-tree" {
			return strings.TrimSpace(string(output)), nil
		}
	}
	return "", errors.New("git wrote no tree")
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
