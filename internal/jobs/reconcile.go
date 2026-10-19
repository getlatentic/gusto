package jobs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/taskindex"
)

// ReconcileResult is what a reconcile or a rebuild found and fixed. Tasks is how many the files hold.
type ReconcileResult struct {
	Tasks   int
	Missing int
	Stale   int
	Gone    int
}

// Fixed is how many rows were written or removed.
func (r ReconcileResult) Fixed() int { return r.Missing + r.Stale + r.Gone }

// repairBatch is how many rows are written between two looks at how far a repair has got.
const repairBatch = 500

// plan is what separates the index from the tasks: the tasks to write a row for, and the rows of tasks that are gone.
type plan struct {
	write []string
	gone  []string
	ReconcileResult
}

// planFor compares the tasks with what the index holds. A row differs when any field of the task differs.
func planFor(tasks map[string]Record, indexed map[string]taskindex.Entry) plan {
	p := plan{ReconcileResult: ReconcileResult{Tasks: len(tasks)}}
	for id, record := range tasks {
		held, found := indexed[id]
		switch {
		case !found:
			p.Missing++
			p.write = append(p.write, id)
		case held.Sum != RowOf(record).Sum():
			p.Stale++
			p.write = append(p.write, id)
		}
	}
	for id := range indexed {
		if _, found := tasks[id]; !found {
			p.Gone++
			p.gone = append(p.gone, id)
		}
	}
	slices.Sort(p.write)
	slices.Sort(p.gone)
	return p
}

// scanned is the tasks on disk by id. A task whose state.json is being written, or cannot be read, is not in it.
func scanned(root string) (map[string]Record, error) {
	listing, err := Scan(root)
	if err != nil {
		return nil, err
	}
	tasks := make(map[string]Record, len(listing))
	for _, record := range listing {
		tasks[record.ID] = record
	}
	return tasks, nil
}

// stateExists says whether a task's state.json is there, even if it cannot be read just now.
func stateExists(root, id string) bool {
	_, err := os.Stat(filepath.Join(root, id, "state.json"))
	return err == nil
}

// Rebuild brings an index up to date with the task directories under root, for a process that does not run the
// tasks: it reads the files, and changes nothing in them. A task that has no seq yet is numbered as a server would
// number it. With fromScratch every row is removed first. It reports how far it has got through progress, which may be nil.
func Rebuild(ctx context.Context, store taskindex.Store, root string, fromScratch bool, progress *taskindex.Progress) (ReconcileResult, error) {
	tasks, err := scanned(root)
	if err != nil {
		return ReconcileResult{}, err
	}
	renumber(tasks)
	if fromScratch {
		if _, err := store.Reset(ctx); err != nil {
			return ReconcileResult{}, err
		}
	}
	indexed, err := store.Indexed(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	p := planFor(tasks, indexed)
	status := taskindex.Status{Building: true, Total: len(tasks), Done: len(tasks) - len(p.write)}
	progress.Set(status)
	defer func() { progress.Set(taskindex.Status{Done: status.Total, Total: status.Total}) }()
	for len(p.write) > 0 {
		if err := ctx.Err(); err != nil {
			return p.ReconcileResult, err
		}
		chunk := p.write[:min(repairBatch, len(p.write))]
		rows := make([]taskindex.Row, len(chunk))
		for at, id := range chunk {
			rows[at] = RowOf(tasks[id])
		}
		if err := store.UpsertMany(ctx, rows); err != nil {
			return p.ReconcileResult, err
		}
		p.write = p.write[len(chunk):]
		status.Done += len(chunk)
		progress.Set(status)
	}
	for _, id := range p.gone {
		if stateExists(root, id) {
			continue
		}
		if err := store.Delete(ctx, id); err != nil {
			return p.ReconcileResult, err
		}
	}
	return p.ReconcileResult, nil
}

// readTask is a task read straight from its directory.
func readTask(root, id string) (Record, error) { return read(job.Dir(filepath.Join(root, id))) }

var errUnreadable = errors.New("the task's state.json cannot be read now")

// diskOp is the change that makes the index say what the disk says of a task this index does not hold in memory.
func diskOp(root, id string) (op, error) {
	record, err := readTask(root, id)
	switch {
	case err == nil:
		return op{row: RowOf(record)}, nil
	case errors.Is(err, fs.ErrNotExist):
		return op{id: id, delete: true}, nil
	}
	return op{}, errUnreadable
}
