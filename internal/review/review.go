// Package review checks a pull request's record from the repository alone.
//
// gusto pr open commits a run's job.json and out/ on a ref beside the pull request,
// with the pull request's commit as its parent. Check fetches that ref with
// git, holds the manifest to the hash the pull request names, re-hashes every
// file against the manifest, rebuilds the workspace from the hashed patch, and
// runs the check. Nothing here talks to GitHub's API or to a gusto server, so the
// check needs nothing from the platform that produced the record.
package review

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/job"
)

// Report is what a check found. Wrong names every file or commit that does not
// agree, and the check runs only when Wrong is empty: code the record does not
// vouch for is not run.
type Report struct {
	Wrong []string
	Files int
	// Claimed is absent when the task had no check, which a turn in a
	// conversation does not.
	Claimed *evidence.Verdict
	Gate    evidence.Verdict
}

// Holds is true when every file matches and the check answers as it did.
func (r Report) Holds() bool {
	if r.Claimed == nil {
		return false
	}
	return len(r.Wrong) == 0 && r.Gate.Passed == r.Claimed.Passed && r.Gate.Code == r.Claimed.Code
}

// Lines is the report as `gusto verify` words it.
func (r Report) Lines() []string {
	if len(r.Wrong) > 0 {
		return r.Wrong
	}
	if !r.Holds() {
		if r.Claimed == nil {
			return []string{"this task had no check, so there is no verdict to check"}
		}
		return []string{fmt.Sprintf("the check now exits %d, and the manifest says %d", r.Gate.Code, r.Claimed.Code)}
	}
	return []string{fmt.Sprintf("%d files match, and the check still exits %d", r.Files, r.Gate.Code)}
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Check fetches the record for the manifest whose sha256 is digest from
// remote, into a task directory at into, and checks it. into must be empty or
// not yet exist. An error means the check could not be made; a finding is in
// the report.
func Check(ctx context.Context, remote, digest, into string) (Report, error) {
	if !sha256Hex.MatchString(digest) {
		return Report{}, fmt.Errorf("%q is not a manifest's sha256: want 64 lowercase hex digits", digest)
	}
	dir := job.Dir(into)
	if err := emptyOrNew(dir); err != nil {
		return Report{}, err
	}
	fetched, err := fetch(ctx, remote, digest, dir)
	if err != nil {
		return Report{}, err
	}
	for _, step := range []func() ([]string, error){
		func() ([]string, error) { return unpack(ctx, fetched, dir) },
		func() ([]string, error) { return held(dir, digest) },
		func() ([]string, error) { return rebuilt(ctx, fetched, dir) },
	} {
		wrong, err := step()
		if err != nil || len(wrong) > 0 {
			return Report{Wrong: wrong}, err
		}
	}
	return gated(dir)
}

// gated runs the gate in the rebuilt workspace and sets its answer beside the
// one the manifest records.
func gated(dir job.Dir) (Report, error) {
	manifest, err := evidence.Read(dir)
	if err != nil {
		return Report{}, err
	}
	spec, err := job.Read(dir)
	if err != nil {
		return Report{}, err
	}
	again, _, err := runner.Ask(spec, dir)
	if err != nil {
		return Report{}, err
	}
	return Report{Files: len(manifest.Files), Claimed: manifest.Gate, Gate: again}, nil
}

func emptyOrNew(dir job.Dir) error {
	listing, err := os.ReadDir(dir.Path())
	if os.IsNotExist(err) {
		return os.MkdirAll(dir.Path(), 0o755)
	}
	if err != nil {
		return err
	}
	if len(listing) > 0 {
		return fmt.Errorf("%s is not empty, and the record is unpacked there", dir.Path())
	}
	return nil
}
