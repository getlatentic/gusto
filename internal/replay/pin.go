package replay

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Pin is the commit a run started from, and the name it was read off.
type Pin struct {
	Commit string
	// By is the reference in the old workspace that still holds the commit as
	// it was when the run cloned.
	By string
	// Committed counts the commits the agent made on top of it. Those are the
	// agent's work, not the repository the task asked for.
	Committed int
}

// Pinned reads the commit a finished run started from out of its workspace.
//
// The workspace's HEAD is not that commit. An agent that commits moves HEAD
// onto its own work, and a rerun pinned there would start with the answer
// already in the tree, or fail to find a commit the repository never had. The
// references a clone writes for the remote do not move when an agent commits:
// refs/remotes/origin/* and refs/tags/* keep what the repository held at clone
// time, and the runner checked the task's ref out from exactly those. So the ref
// is resolved against them, and a ref that is a commit id is its own answer.
func Pinned(workspace, ref string) (Pin, error) {
	for _, name := range candidates(ref) {
		commit, err := git(workspace, "rev-parse", "--verify", "--quiet", "--end-of-options", name+"^{commit}")
		if err != nil {
			continue
		}
		committed, err := committedSince(workspace, commit)
		if err != nil {
			return Pin{}, err
		}
		return Pin{Commit: commit, By: name, Committed: committed}, nil
	}
	return Pin{}, fmt.Errorf("the workspace in %s no longer says which commit %q named when the task cloned it", workspace, ref)
}

// candidates are the names a clone leaves for the ref, in the order the runner's
// checkout would have found them. HEAD becomes refs/remotes/origin/HEAD, the
// remote's default branch as the clone saw it.
func candidates(ref string) []string {
	names := []string{"refs/remotes/origin/" + ref, "refs/tags/" + ref}
	if commitID.MatchString(ref) {
		names = append(names, ref)
	}
	return names
}

var commitID = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// committedSince counts what HEAD holds that the pin does not: the commits the
// harness made. Its diff.patch shows none of them, since the runner diffs the
// working tree.
func committedSince(workspace, commit string) (int, error) {
	count, err := git(workspace, "rev-list", "--count", commit+"..HEAD")
	if err != nil {
		return 0, err
	}
	var n int
	_, err = fmt.Sscanf(count, "%d", &n)
	return n, err
}

// git reads; nothing it runs here writes to the old workspace, which is
// evidence.
func git(at string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", at}, args...)...)
	var output, complaint bytes.Buffer
	command.Stdout = &output
	command.Stderr = &complaint
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(complaint.String()))
	}
	return strings.TrimSpace(output.String()), nil
}

// Resolve pins a ref before any run has cloned it, the same way Pinned reads it
// off a finished run: from a clone's record of the remote. The clone takes no
// working tree and no file contents, and is gone when Resolve returns, so tasks
// that must start from one commit can each clone their own workspace at it.
func Resolve(repo, ref string) (Pin, error) {
	scratch, err := os.MkdirTemp("", "gusto-pin-")
	if err != nil {
		return Pin{}, err
	}
	defer os.RemoveAll(scratch)
	if _, err := git(scratch, "clone", "--quiet", "--no-checkout", "--filter=blob:none", repo, "clone"); err != nil {
		return Pin{}, err
	}
	pin, err := Pinned(filepath.Join(scratch, "clone"), ref)
	pin.Committed = 0
	return pin, err
}
