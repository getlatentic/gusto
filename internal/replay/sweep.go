package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/job"
)

// Run does the work of one prepared replay. Its error is for a run that could
// not happen; the verdict is read from the record it leaves.
type Run func(job.Dir) error

// Row is one task in a sweep: what it was, and what it is now.
type Row struct {
	Old    job.Dir
	Replay job.Dir
	Before evidence.Manifest
	After  evidence.Manifest
	// Err is set when the rerun could not be made, could not run, or left no
	// verdict. Then After is empty and Change says nothing about the harness.
	Err error
}

// Sweep reruns every task, one after the other, and returns a row each. One task
// that cannot be rerun is a row with an error rather than the end of the
// sweep, so a single broken directory does not hide what happened to the rest.
func Sweep(olds []job.Dir, parent string, run Run) []Row {
	rows := make([]Row, 0, len(olds))
	for _, old := range olds {
		rows = append(rows, once(old, parent, run))
	}
	return rows
}

func once(old job.Dir, parent string, run Run) Row {
	row := Row{Old: old}
	row.Before, row.Err = Known(old)
	if row.Err != nil {
		return row
	}
	row.Replay, row.Err = Prepare(old, parent)
	if row.Err != nil {
		return row
	}
	if row.Err = run(row.Replay); row.Err != nil {
		return row
	}
	row.After, row.Err = verdictOf(row.Replay)
	return row
}

// verdictOf is the replay's own verdict, read from its evidence rather than
// from an exit code, because only the manifest says which harness answered.
func verdictOf(dir job.Dir) (evidence.Manifest, error) {
	manifest, err := evidence.Read(dir)
	if err != nil {
		return evidence.Manifest{}, fmt.Errorf("the rerun in %s left no manifest: %w", dir, err)
	}
	if manifest.Gate == nil {
		return evidence.Manifest{}, fmt.Errorf("the rerun in %s left no verdict", dir)
	}
	return manifest, nil
}

// Expand turns what a person named into jobs. A directory holding job.json is a
// task. Any other directory is a set of tasks, and every child holding job.json is
// one, in name order, except the reruns among them: a sweep repeats originals,
// and a second sweep over the same directory should not rerun the first one's
// replays.
func Expand(paths []string) ([]job.Dir, error) {
	var jobs []job.Dir
	for _, path := range paths {
		found, err := expandOne(path)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, found...)
	}
	return jobs, nil
}

func expandOne(path string) ([]job.Dir, error) {
	if isJob(path) {
		return []job.Dir{job.Dir(path)}, nil
	}
	listing, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var jobs []job.Dir
	for _, entry := range listing {
		child := job.Dir(filepath.Join(path, entry.Name()))
		if entry.IsDir() && isJob(child.Path()) && !isReplay(child) {
			jobs = append(jobs, child)
		}
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("%s is not a task and holds none", path)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i] < jobs[j] })
	return jobs, nil
}

func isJob(path string) bool {
	_, err := os.Stat(job.Dir(path).Spec())
	return err == nil
}

func isReplay(dir job.Dir) bool {
	_, err := os.Stat(dir.OutFile(MarkFile))
	return err == nil
}
