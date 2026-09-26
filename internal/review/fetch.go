package review

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// evidenceRef is where haas ship leaves the evidence for a manifest.
func evidenceRef(digest string) string {
	return "refs/haas/evidence/" + digest
}

// fetched names the three commits a check reads: the evidence, the pull
// request's commit under it, and the commit the job ran against under that.
type fetched struct {
	workspace string
	evidence  string
	change    string
	at        string
}

// fetch makes the job's workspace a repository and fetches the evidence ref
// into it, three commits deep: that is every commit the check reads, so a
// large repository's history stays on the remote. The reviewer's own git
// credentials reach the remote, so the check sees what the reviewer may see.
func fetch(ctx context.Context, remote, digest string, dir job.Dir) (fetched, error) {
	workspace := dir.Workspace()
	ref := evidenceRef(digest)
	if _, err := git(ctx, "init", "--quiet", workspace); err != nil {
		return fetched{}, err
	}
	if _, err := git(ctx, "-C", workspace, "fetch", "--quiet", "--depth", "3", remote, ref+":"+ref); err != nil {
		return fetched{}, fmt.Errorf("%s holds no evidence for that manifest: %w", remote, err)
	}
	evidence, err := named(ctx, workspace, ref+"^{commit}", "the evidence ref names no commit")
	if err != nil {
		return fetched{}, err
	}
	change, err := named(ctx, workspace, evidence+"^1", "the evidence commit has no parent, so it vouches for no change")
	if err != nil {
		return fetched{}, err
	}
	at, err := named(ctx, workspace, change+"^1", "the pull request's commit has no parent, so it names no commit the job ran against")
	if err != nil {
		return fetched{}, err
	}
	return fetched{workspace: workspace, evidence: evidence, change: change, at: at}, nil
}

// named is the commit a revision names, or what it means that there is none.
func named(ctx context.Context, workspace, revision, lack string) (string, error) {
	commit, err := git(ctx, "-C", workspace, "rev-parse", "--verify", "--quiet", revision)
	if err != nil {
		return "", fmt.Errorf("%s: %w", lack, err)
	}
	return commit, nil
}

// git returns what git printed on stdout, trimmed, and puts stderr in the
// error, where a person needs it.
func git(ctx context.Context, args ...string) (string, error) {
	output, err := gitBytes(ctx, args...)
	return strings.TrimSpace(string(output)), err
}

// gitBytes returns stdout as git wrote it, for file contents whose every byte
// is hashed.
func gitBytes(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	var output, complaint bytes.Buffer
	command.Stdout = &output
	command.Stderr = &complaint
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(complaint.String()))
	}
	return output.Bytes(), nil
}
