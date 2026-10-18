package ship

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/repoaccess"
)

// Git pushes by shelling out to git in a scratch repository of its own, so the
// task's workspace, which is record, is never touched.
type Git struct {
	// Token, when set, is sent to Host and to no other.
	Token string
	// Host is the forge's address, https://github.com when empty.
	Host string
}

// Push makes the change and record commits again and pushes both in one push,
// the branch and the record ref together, so a branch never reaches the
// remote without the record behind it. The pull request's body links the
// record commit by name, so commits that come out different from the plan
// stop the push.
func (g Git) Push(ctx context.Context, plan Plan) error {
	dir := job.Dir(plan.Job)
	return inScratch(ctx, dir, plan.When, func(s scratch) error {
		made, err := s.made(ctx, dir, plan)
		if err != nil {
			return err
		}
		if made != plan.Commits {
			return fmt.Errorf("the commits came out as %s and %s, and the plan named %s and %s",
				made.Change, made.Evidence, plan.Commits.Change, plan.Commits.Evidence)
		}
		_, err = run(ctx, g.env(), "-C", s.path, "push", "--quiet", plan.Remote,
			made.Change+":refs/heads/"+plan.Branch, made.Evidence+":"+plan.Evidence)
		return err
	})
}

// authored names gusto as the author and dates the commits at the end of the
// run, so shipping the same run twice makes the same commits and the second
// push has nothing to add.
func authored(when int64) []string {
	date := fmt.Sprintf("@%d +0000", when)
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=gusto", "GIT_AUTHOR_EMAIL=gusto@localhost", "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=gusto", "GIT_COMMITTER_EMAIL=gusto@localhost", "GIT_COMMITTER_DATE="+date,
	)
}

// env carries the token by environment rather than argument, where any process
// listing would show it, and scopes the header to the forge.
func (g Git) env() []string {
	env := os.Environ()
	if g.Token == "" {
		return env
	}
	return repoaccess.WithToken(env, or(g.Host, "https://github.com"), g.Token)
}

// ranAgainst is the commit the workspace was checked out at. The patch is a
// diff against it, so it is the one commit the patch is sure to apply to.
func ranAgainst(dir job.Dir) (string, error) {
	at, err := run(context.Background(), nil, "-C", dir.Workspace(), "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", fmt.Errorf("the workspace does not say which commit the task ran against: %w", err)
	}
	return at, nil
}

// baseBranch is the branch the pull request targets: the one asked for, else
// the job's ref when the remote has it as a branch, else the remote's default.
// One ls-remote answers both questions and changes nothing on the remote.
func baseBranch(remote, ref, asked string) (string, error) {
	if asked != "" {
		return asked, nil
	}
	listing, err := run(context.Background(), nil, "ls-remote", "--symref", remote, "HEAD", "refs/heads/"+ref)
	if err != nil {
		return "", err
	}
	branches, fallback := advertised(listing)
	if branches[ref] {
		return ref, nil
	}
	if fallback == "" {
		return "", fmt.Errorf("%s names no default branch; name one with -base", remote)
	}
	return fallback, nil
}

// advertised reads ls-remote's answer: the branches it listed, and the branch
// its HEAD points at.
func advertised(listing string) (map[string]bool, string) {
	branches, fallback := map[string]bool{}, ""
	for _, line := range strings.Split(listing, "\n") {
		if target, found := strings.CutPrefix(line, "ref: refs/heads/"); found {
			fallback = strings.TrimSuffix(target, "\tHEAD")
			continue
		}
		if _, name, found := strings.Cut(line, "\trefs/heads/"); found {
			branches[name] = true
		}
	}
	return branches, fallback
}

// run returns what git printed on stdout, and puts stderr in the error, where a
// person needs it.
func run(ctx context.Context, env []string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = env
	var output, complaint bytes.Buffer
	command.Stdout = &output
	command.Stderr = &complaint
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(complaint.String()))
	}
	return strings.TrimSpace(output.String()), nil
}
