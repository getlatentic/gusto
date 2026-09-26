package ship

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// Options overrides what the job would otherwise decide. Every field may be
// empty, and then the plan is read off the job alone.
type Options struct {
	// Remote is where the branch is pushed. The job's repository by default.
	Remote string
	// GitHub is owner/name. Read off the remote by default.
	GitHub string
	// Base is the branch the pull request targets. The job's ref when the
	// remote has a branch by that name, and the remote's default otherwise.
	Base string
}

// Plan is everything shipping will do, decided before anything is done. The
// same job gives the same plan, so a dry run shows exactly what a real run does.
type Plan struct {
	Job             string
	Remote          string
	Branch          string
	Evidence        string
	At              string
	Patch           string
	Owner           string
	Name            string
	Base            string
	Title           string
	Body            string
	Message         string
	EvidenceMessage string
	Manifest        string
	Commits         Commits
	When            int64
}

// Prepare refuses a job that must not ship, and plans one that may.
func Prepare(dir job.Dir, options Options) (Plan, error) {
	manifest, err := Shippable(dir)
	if err != nil {
		return Plan{}, err
	}
	spec, err := job.Read(dir)
	if err != nil {
		return Plan{}, err
	}
	digest, err := fileHash(dir.OutFile("manifest.json"))
	if err != nil {
		return Plan{}, err
	}
	return planned(dir, spec, manifest, digest, options)
}

func planned(dir job.Dir, spec job.Spec, manifest evidence.Manifest, digest string, options Options) (Plan, error) {
	at, err := ranAgainst(dir)
	if err != nil {
		return Plan{}, err
	}
	remote := located(or(options.Remote, spec.Repo))
	owner, name, err := repository(options.GitHub, remote)
	if err != nil {
		return Plan{}, err
	}
	base, err := baseBranch(remote, spec.Ref, options.Base)
	if err != nil {
		return Plan{}, err
	}
	patch, err := filepath.Abs(dir.OutFile("diff.patch"))
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Job:             dir.Path(),
		Remote:          remote,
		Branch:          branchName(dir, digest),
		Evidence:        EvidenceRef(digest),
		At:              at,
		Patch:           patch,
		Owner:           owner,
		Name:            name,
		Base:            base,
		Manifest:        digest,
		When:            manifest.AtUnix,
		EvidenceMessage: evidenceMessage(dir, digest),
	}
	return described(dir, plan, record{spec: spec, manifest: manifest, digest: digest, at: at})
}

// described makes the commits to learn their names, then writes what the pull
// request says, which links the evidence commit by name.
func described(dir job.Dir, plan Plan, record record) (Plan, error) {
	plan.Title = record.title()
	plan.Message = record.message(dir)
	commits, err := planCommits(context.Background(), dir, plan)
	if err != nil {
		return Plan{}, err
	}
	plan.Commits = commits
	record.links = links{owner: plan.Owner, name: plan.Name, remote: plan.Remote, ref: plan.Evidence, commits: commits}
	plan.Body = record.body()
	return plan, nil
}

// Pull is the request the forge is sent.
func (p Plan) Pull() Pull {
	return Pull{Owner: p.Owner, Name: p.Name, Head: p.Branch, Base: p.Base, Title: p.Title, Body: p.Body}
}

// branchName is the job's directory name and the start of its manifest's hash.
// The name tells a person which job it is, and the hash tells two runs of the
// same directory apart, so shipping the same run twice names the same branch.
func branchName(dir job.Dir, digest string) string {
	return "haas/" + slug(filepath.Base(filepath.Clean(dir.Path()))) + "-" + digest[:12]
}

func slug(text string) string {
	var out strings.Builder
	for _, letter := range strings.ToLower(text) {
		if strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789", letter) {
			out.WriteRune(letter)
			continue
		}
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
			out.WriteRune('-')
		}
	}
	cut := strings.Trim(out.String(), "-")
	if len(cut) > 48 {
		cut = strings.Trim(cut[:48], "-")
	}
	return or(cut, "job")
}

// repository reads owner/name from what the person named, or from the remote.
func repository(named, remote string) (string, string, error) {
	path := or(named, githubPath(remote))
	owner, name, found := strings.Cut(path, "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("cannot tell which GitHub repository %s is; name it with -github owner/name", remote)
	}
	return owner, name, nil
}

// githubPath is owner/name for the three ways a GitHub remote is written, and
// empty for anything else.
func githubPath(remote string) string {
	for _, prefix := range []string{"https://github.com/", "ssh://git@github.com/", "git@github.com:"} {
		if rest, found := strings.CutPrefix(remote, prefix); found {
			return strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
		}
	}
	return ""
}

// located makes a local path absolute, because git runs in a clone somewhere
// else and a relative path would point nowhere from there. A URL is left alone.
func located(where string) string {
	if _, err := os.Stat(where); err != nil {
		return where
	}
	if absolute, err := filepath.Abs(where); err == nil {
		return absolute
	}
	return where
}

func fileHash(path string) (string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:]), nil
}

func or(value, otherwise string) string {
	if value != "" {
		return value
	}
	return otherwise
}
