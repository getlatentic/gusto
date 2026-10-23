package review

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/getlatentic/gusto/pkg/job"
)

// rebuilt checks out the commit the job ran against and applies the hashed
// patch to it, which is the workspace the gate passed in. The pull request's
// commit has to hold the same tree, or the code under review is not the code
// whose gate passed.
func rebuilt(ctx context.Context, found fetched, dir job.Dir) ([]string, error) {
	patch, err := filepath.Abs(dir.OutFile("diff.patch"))
	if err != nil {
		return nil, err
	}
	if _, err := git(ctx, "-C", found.workspace, "checkout", "--quiet", "--detach", found.at); err != nil {
		return nil, err
	}
	if _, err := git(ctx, "-C", found.workspace, "apply", "--index", patch); err != nil {
		return []string{fmt.Sprintf("diff.patch does not apply on %s, the parent of the pull request's commit: %v", found.at, err)}, nil
	}
	tree, err := git(ctx, "-C", found.workspace, "write-tree")
	if err != nil {
		return nil, err
	}
	want, err := git(ctx, "-C", found.workspace, "rev-parse", found.change+"^{tree}")
	if err != nil {
		return nil, err
	}
	if tree != want {
		return []string{fmt.Sprintf("the pull request's commit %s is not diff.patch on %s", found.change, found.at)}, nil
	}
	return nil, nil
}
