package review

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/getlatentic/gusto/pkg/task"
)

// recordRef is where gusto pr open leaves the record for a manifest.
func recordRef(digest string) string {
	return "refs/gusto/evidence/" + digest
}

// fetched names the three commits a check reads: the record, the pull
// request's commit under it, and the commit the task ran against under that.
type fetched struct {
	workingFolder string
	record        string
	change        string
	at            string
}

// fetch makes the task's working folder a repository and fetches the record ref
// into it, three commits deep: that is every commit the check reads, so a
// large repository's history stays on the remote. The reviewer's own git
// credentials reach the remote, so the check sees what the reviewer may see.
func fetch(ctx context.Context, remote, digest string, dir task.Dir) (fetched, error) {
	workingFolder := dir.WorkingFolder()
	ref := recordRef(digest)
	if _, err := git(ctx, "init", "--quiet", workingFolder); err != nil {
		return fetched{}, err
	}
	if _, err := git(ctx, "-C", workingFolder, "fetch", "--quiet", "--depth", "3", remote, ref+":"+ref); err != nil {
		return fetched{}, fmt.Errorf("%s holds no record for that manifest: %w", remote, err)
	}
	record, err := named(ctx, workingFolder, ref+"^{commit}", "the record ref names no commit")
	if err != nil {
		return fetched{}, err
	}
	change, err := named(ctx, workingFolder, record+"^1", "the record commit has no parent, so it vouches for no change")
	if err != nil {
		return fetched{}, err
	}
	at, err := named(ctx, workingFolder, change+"^1", "the pull request's commit has no parent, so it names no commit the task ran against")
	if err != nil {
		return fetched{}, err
	}
	return fetched{workingFolder: workingFolder, record: record, change: change, at: at}, nil
}

// named is the commit a revision names, or what it means that there is none.
func named(ctx context.Context, workingFolder, revision, lack string) (string, error) {
	commit, err := git(ctx, "-C", workingFolder, "rev-parse", "--verify", "--quiet", revision)
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
