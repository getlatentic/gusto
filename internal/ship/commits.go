package ship

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/getlatentic/gusto/pkg/job"
)

// Commits are the two a pull request rests on. Change is the patch on the
// commit the task ran against, and it is the pull request's head. Record holds
// job.json and out/ exactly as the run left them, with Change as its parent, so
// whoever fetches the record gets the code it vouches for with it.
type Commits struct {
	Change   string
	Evidence string
}

// EvidenceRef is where the record commit lives on the remote. It sits outside
// refs/heads and refs/tags, so it is not a branch anyone reviews, merges or
// builds, and the change under review stays the patch and nothing else.
func EvidenceRef(digest string) string {
	return "refs/gusto/evidence/" + digest
}

// scratch is a repository of its own that borrows the workspace's objects. The
// commits are made there, so the workspace, which is evidence, is never written
// to, and nothing has to be cloned from the remote to make them.
type scratch struct {
	path string
	env  []string
}

// inScratch makes a scratch repository, hands it over, and removes it.
func inScratch(ctx context.Context, dir job.Dir, when int64, do func(scratch) error) error {
	path, err := os.MkdirTemp("", "gusto-ship-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(path)
	if _, err := run(ctx, nil, "clone", "--quiet", "--shared", "--no-checkout", dir.Workspace(), path); err != nil {
		return err
	}
	return do(scratch{path: path, env: authored(when)})
}

// made builds both commits. The author, the dates and the messages are fixed
// by the run, so the same run makes the same two commits on any machine, and a
// plan can name them before anything is pushed.
func (s scratch) made(ctx context.Context, dir job.Dir, plan Plan) (Commits, error) {
	change, err := s.change(ctx, plan)
	if err != nil {
		return Commits{}, err
	}
	evidence, err := s.evidence(ctx, dir, change, plan.EvidenceMessage)
	if err != nil {
		return Commits{}, err
	}
	return Commits{Change: change, Evidence: evidence}, nil
}

// change applies the patch to the index alone. A patch that no longer applies
// at the commit the job ran against stops it before anything leaves the
// machine.
func (s scratch) change(ctx context.Context, plan Plan) (string, error) {
	if err := s.git(ctx, "read-tree", plan.At); err != nil {
		return "", err
	}
	if err := s.git(ctx, "apply", "--cached", plan.Patch); err != nil {
		return "", err
	}
	return s.commit(ctx, plan.At, plan.Message)
}

// evidence stages job.json and every file in out/ under the names a job
// directory gives them, and commits them on top of the change.
func (s scratch) evidence(ctx context.Context, dir job.Dir, change, message string) (string, error) {
	if err := s.git(ctx, "read-tree", "--empty"); err != nil {
		return "", err
	}
	files, err := evidenceFiles(dir)
	if err != nil {
		return "", err
	}
	for _, name := range files {
		if err := s.stage(ctx, filepath.Join(dir.Path(), name), name); err != nil {
			return "", err
		}
	}
	return s.commit(ctx, change, message)
}

func (s scratch) stage(ctx context.Context, from, name string) error {
	absolute, err := filepath.Abs(from)
	if err != nil {
		return err
	}
	blob, err := run(ctx, s.env, "-C", s.path, "hash-object", "-w", "--", absolute)
	if err != nil {
		return err
	}
	return s.git(ctx, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+name)
}

// commit never signs, because a signature carries its own time and would make
// the same run a different commit each time it ships.
func (s scratch) commit(ctx context.Context, parent, message string) (string, error) {
	tree, err := run(ctx, s.env, "-C", s.path, "write-tree")
	if err != nil {
		return "", err
	}
	return run(ctx, s.env, "-C", s.path, "commit-tree", "--no-gpg-sign", tree, "-p", parent, "-m", message)
}

func (s scratch) git(ctx context.Context, args ...string) error {
	_, err := run(ctx, s.env, append([]string{"-C", s.path}, args...)...)
	return err
}

// evidenceFiles is job.json and every file in out/, which is what the manifest
// hashes, and the manifest itself. The workspace is left out: the change
// commit already carries the code, and a reviewer rebuilds it from the patch.
func evidenceFiles(dir job.Dir) ([]string, error) {
	listing, err := os.ReadDir(dir.Out())
	if err != nil {
		return nil, err
	}
	files := []string{"job.json"}
	for _, entry := range listing {
		if entry.Type().IsRegular() {
			files = append(files, "out/"+entry.Name())
		}
	}
	sort.Strings(files[1:])
	return files, nil
}

// planCommits makes the commits in a scratch repository to learn their names,
// and keeps nothing.
func planCommits(ctx context.Context, dir job.Dir, plan Plan) (Commits, error) {
	var commits Commits
	err := inScratch(ctx, dir, plan.When, func(s scratch) error {
		var err error
		commits, err = s.made(ctx, dir, plan)
		return err
	})
	if err != nil {
		return Commits{}, fmt.Errorf("the patch and its record cannot be committed: %w", err)
	}
	return commits, nil
}
