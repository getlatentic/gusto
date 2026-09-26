package ship

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// Git pushes by shelling out to git in a clone of its own, so the job's
// workspace, which is evidence, is never touched.
type Git struct {
	// Token, when set, is sent to github.com and to no other host.
	Token string
}

// Push clones the job's repository, puts the patch on a new branch at the
// commit the job ran against, and pushes that branch alone. A patch that no
// longer applies there stops it before anything leaves the machine.
func (g Git) Push(ctx context.Context, plan Plan) error {
	work, err := os.MkdirTemp("", "haas-ship-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	env := g.env(plan.When)
	for _, args := range [][]string{
		{"clone", "--quiet", "--no-checkout", plan.Source, work},
		{"-C", work, "checkout", "--quiet", "-b", plan.Branch, plan.At},
		{"-C", work, "apply", "--index", plan.Patch},
		{"-C", work, "commit", "--quiet", "--no-verify", "-m", plan.Message},
		{"-C", work, "push", "--quiet", plan.Remote, "HEAD:refs/heads/" + plan.Branch},
	} {
		if _, err := run(ctx, env, args...); err != nil {
			return err
		}
	}
	return nil
}

// env names haas as the author and dates the commit at the end of the run, so
// shipping the same run twice makes the same commit and the second push has
// nothing to add. The token goes in by environment rather than argument, where
// any process listing would show it, and the header is scoped to github.com.
func (g Git) env(when int64) []string {
	date := fmt.Sprintf("@%d +0000", when)
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=haas", "GIT_AUTHOR_EMAIL=haas@localhost", "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=haas", "GIT_COMMITTER_EMAIL=haas@localhost", "GIT_COMMITTER_DATE="+date,
	)
	if g.Token == "" {
		return env
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + g.Token))
	return append(env, "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+basic)
}

// ranAgainst is the commit the workspace was checked out at. The patch is a
// diff against it, so it is the one commit the patch is sure to apply to.
func ranAgainst(dir job.Dir) (string, error) {
	at, err := run(context.Background(), nil, "-C", dir.Workspace(), "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", fmt.Errorf("the workspace does not say which commit the job ran against: %w", err)
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
