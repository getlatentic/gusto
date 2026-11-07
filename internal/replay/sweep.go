package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/task"
)

// Run does the work of one prepared replay. Its error is for a run that could
// not happen; the verdict is read from the record it leaves.
type Run func(task.Dir) error

// Row is one task in a sweep: what it was, and what it is now.
type Row struct {
	Old    task.Dir
	Replay task.Dir
	Before evidence.Manifest
	After  evidence.Manifest
	// Err is set when the rerun could not be made, could not run, or left no
	// verdict. Then After is empty and Change says nothing about the harness.
	Err error
}

// Sweep reruns every task, one after the other, and returns a row each. One task
// that cannot be rerun is a row with an error rather than the end of the
// sweep, so a single broken directory does not hide what happened to the rest.
func Sweep(olds []task.Dir, parent string, run Run) []Row {
	rows := make([]Row, 0, len(olds))
	for _, old := range olds {
		rows = append(rows, once(old, parent, run))
	}
	return rows
}

func once(old task.Dir, parent string, run Run) Row {
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
func verdictOf(dir task.Dir) (evidence.Manifest, error) {
	manifest, err := evidence.Read(dir)
	if err != nil {
		return evidence.Manifest{}, fmt.Errorf("the rerun in %s left no manifest: %w", dir, err)
	}
	if manifest.Gate == nil {
		return evidence.Manifest{}, fmt.Errorf("the rerun in %s left no verdict", dir)
	}
	return manifest, nil
}

// Expand turns what a person named into tasks. A directory holding job.json is a
// task. Any other directory is a set of tasks, and every child holding job.json is
// one, in name order, except the reruns among them: a sweep repeats originals,
// and a second sweep over the same directory should not rerun the first one's
// replays.
func Expand(paths []string) ([]task.Dir, error) {
	var tasks []task.Dir
	for _, path := range paths {
		found, err := expandOne(path)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, found...)
	}
	return tasks, nil
}

func expandOne(path string) ([]task.Dir, error) {
	if isTask(path) {
		return []task.Dir{task.Dir(path)}, nil
	}
	listing, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var tasks []task.Dir
	for _, entry := range listing {
		child := task.Dir(filepath.Join(path, entry.Name()))
		if entry.IsDir() && isTask(child.Path()) && !isReplay(child) {
			tasks = append(tasks, child)
		}
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("%s is not a task and holds none", path)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i] < tasks[j] })
	return tasks, nil
}

func isTask(path string) bool {
	_, err := os.Stat(task.Dir(path).Spec())
	return err == nil
}

func isReplay(dir task.Dir) bool {
	_, err := os.Stat(dir.OutFile(MarkFile))
	return err == nil
}
