package tasks

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/pkg/task"
)

// ReconcileResult is what a reconcile or a rebuild found and fixed. Tasks is how many the files hold.
type ReconcileResult struct {
	Tasks   int
	Missing int
	Stale   int
	Gone    int
	// Ahead is how many rows differ from their task and are further on than it: a task read from files older than the row,
	// such as files put back from a backup. They are left as they are, since a server that is running writes rows that are
	// ahead of the files it has not yet written. A rebuild that Forces writes them.
	Ahead int
}

// Fixed is how many rows were written or removed.
func (r ReconcileResult) Fixed() int { return r.Missing + r.Stale + r.Gone }

// repairBatch is how many rows are written between two looks at how far a repair has got.
const repairBatch = 500

// plan is what separates the index from the tasks: the tasks to write a row for, and the rows of tasks that are gone.
type plan struct {
	write []string
	// restore is the rows that are ahead of their task and are to be written anyway.
	restore []string
	gone    []string
	ReconcileResult
}

// planFor compares the tasks with what the index holds. A row differs when any field of the task differs. A row that differs
// and is ahead of its task is left unless force is set.
func planFor(tasks map[string]Record, indexed map[string]taskindex.Entry, force bool) plan {
	p := plan{ReconcileResult: ReconcileResult{Tasks: len(tasks)}}
	for id, record := range tasks {
		held, found := indexed[id]
		switch {
		case !found:
			p.Missing++
			p.write = append(p.write, id)
		case held.Sum != RowOf(record).Sum():
			p.differs(id, RowOf(record).ReplacesEntry(held), force)
		}
	}
	for id := range indexed {
		if _, found := tasks[id]; !found {
			p.Gone++
			p.gone = append(p.gone, id)
		}
	}
	slices.Sort(p.write)
	slices.Sort(p.restore)
	slices.Sort(p.gone)
	return p
}

// differs files a row that is not what its task says under the list it is written from, or leaves it as Ahead.
func (p *plan) differs(id string, behind, force bool) {
	switch {
	case behind:
		p.Stale++
		p.write = append(p.write, id)
	case force:
		p.Stale++
		p.restore = append(p.restore, id)
	default:
		p.Ahead++
	}
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
// A row that is ahead of its task is left, and counted as Ahead: see RebuildWith.
func Rebuild(ctx context.Context, store taskindex.Store, root string, fromScratch bool, progress *taskindex.Progress) (ReconcileResult, error) {
	return RebuildWith(ctx, store, root, RebuildOptions{FromScratch: fromScratch}, progress)
}

// RebuildOptions says how far a rebuild goes.
type RebuildOptions struct {
	// FromScratch removes every row first.
	FromScratch bool
	// Force writes a row that is ahead of its task, which a rebuild otherwise leaves. It is for files that were put back from
	// a backup and are the truth. A server that is running holds rows ahead of its files for a moment, so Force is for when
	// none is writing.
	Force bool
}

// RebuildWith is Rebuild with the options given.
func RebuildWith(ctx context.Context, store taskindex.Store, root string, opts RebuildOptions, progress *taskindex.Progress) (ReconcileResult, error) {
	tasks, err := scanned(root)
	if err != nil {
		return ReconcileResult{}, err
	}
	renumber(tasks)
	if opts.FromScratch {
		if _, err := store.Reset(ctx); err != nil {
			return ReconcileResult{}, err
		}
	}
	indexed, err := store.Indexed(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	p := planFor(tasks, indexed, opts.Force)
	status := taskindex.Status{Building: true, Total: len(tasks), Done: len(tasks) - len(p.write) - len(p.restore)}
	progress.Set(status)
	defer func() { progress.Set(taskindex.Status{Done: status.Total, Total: status.Total}) }()
	written := func(n int) {
		status.Done += n
		progress.Set(status)
	}
	if err := writeRows(ctx, tasks, p.write, store.UpsertMany, written); err != nil {
		return p.ReconcileResult, err
	}
	if err := writeRows(ctx, tasks, p.restore, store.Restore, written); err != nil {
		return p.ReconcileResult, err
	}
	if err := removeGone(ctx, store, root, p.gone); err != nil {
		return p.ReconcileResult, err
	}
	return p.ReconcileResult, nil
}

// writeRows writes the rows of the tasks named, a batch at a time, with put, and says after each batch how many it wrote.
func writeRows(ctx context.Context, tasks map[string]Record, ids []string,
	put func(context.Context, []taskindex.Row) error, written func(int)) error {
	for len(ids) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := ids[:min(repairBatch, len(ids))]
		rows := make([]taskindex.Row, len(chunk))
		for at, id := range chunk {
			rows[at] = RowOf(tasks[id])
		}
		if err := put(ctx, rows); err != nil {
			return err
		}
		ids = ids[len(chunk):]
		written(len(chunk))
	}
	return nil
}

// removeGone removes the rows of the tasks named, unless a task's state.json has appeared since the scan.
func removeGone(ctx context.Context, store taskindex.Store, root string, ids []string) error {
	for _, id := range ids {
		if stateExists(root, id) {
			continue
		}
		if err := store.Delete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// readTask is a task read straight from its directory.
func readTask(root, id string) (Record, error) { return read(task.Dir(filepath.Join(root, id))) }

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
