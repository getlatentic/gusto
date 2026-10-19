package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/getlatentic/gusto/internal/taskindex"
)

// Reconcile makes the store say what the task directories say: it writes a row for a task that has none or whose
// row differs in any field, and removes the row of a task whose directory is gone. A task this server holds is
// written as the server holds it, which is never behind its files, so a task that changes while Reconcile runs is
// not set back. The changes go through the mirror's queue, behind those already in it, and Reconcile waits for
// them. With no mirror set it writes to store itself. It runs at start, and again when the mirror reports trouble.
// It returns an error when a change was not written, and a later run repairs it.
func (i *Index) Reconcile(ctx context.Context, store taskindex.Store, progress *taskindex.Progress) (ReconcileResult, error) {
	progress.Set(taskindex.Status{Building: true})
	onDisk, err := scanned(i.root)
	if err != nil {
		return ReconcileResult{}, err
	}
	indexed, err := store.Indexed(ctx)
	if err != nil {
		return ReconcileResult{}, err
	}
	p := planFor(i.overlaid(onDisk), indexed)
	status := taskindex.Status{Building: true, Total: p.Tasks, Done: p.Tasks - len(p.write)}
	progress.Set(status)
	defer func() { progress.Set(taskindex.Status{Done: status.Total, Total: status.Total}) }()
	out, finished := i.repairer(store)
	defer finished()
	before := out.stats()
	written := func(n int) {
		status.Done += n
		progress.Set(status)
	}
	if err := i.repairAll(ctx, out, p.write, written); err != nil {
		return p.ReconcileResult, err
	}
	if err := i.repairAll(ctx, out, p.gone, func(int) {}); err != nil {
		return p.ReconcileResult, err
	}
	if after := out.stats(); after != before {
		return p.ReconcileResult, fmt.Errorf("%d change(s) to the task index were not written",
			after.Dropped+after.Failed-before.Dropped-before.Failed)
	}
	return p.ReconcileResult, nil
}

// overlaid is the tasks on disk with the ones this index holds in their place: memory is never behind the files,
// and holds what a task is numbered as.
func (i *Index) overlaid(onDisk map[string]Record) map[string]Record {
	i.guard.Lock()
	defer i.guard.Unlock()
	for id, record := range i.records {
		onDisk[id] = record
	}
	return onDisk
}

// repairer is the queue repairs go through: the mirror's, or when there is none one over the store for the
// length of the run.
func (i *Index) repairer(store taskindex.Store) (*pump, func()) {
	i.guard.Lock()
	p := i.pump
	i.guard.Unlock()
	if p != nil {
		return p, func() {}
	}
	own := newPump(store, mirrorQueue)
	return own, own.close
}

// repairAll queues the repairs of the tasks a batch at a time and waits for each batch to be handled.
func (i *Index) repairAll(ctx context.Context, out *pump, ids []string, handled func(int)) error {
	for start := 0; start < len(ids); start += repairBatch {
		chunk := ids[start:min(start+repairBatch, len(ids))]
		for _, id := range chunk {
			if err := i.repair(ctx, out, id); err != nil {
				return err
			}
		}
		if err := out.flush(ctx); err != nil {
			return err
		}
		handled(len(chunk))
	}
	return nil
}

// repair queues the change that makes a task's row what the task is now, and waits for room in the queue, which
// a task changing never does.
func (i *Index) repair(ctx context.Context, out *pump, id string) error {
	for !i.tryRepair(out, id) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(spaceCheckWait):
		}
	}
	return nil
}

// tryRepair reads the task under the guard, so that what it queues is not older than any change queued after
// it. It says false only when the queue is full. A task that cannot be read now is left for the next run.
func (i *Index) tryRepair(out *pump, id string) bool {
	i.guard.Lock()
	defer i.guard.Unlock()
	if record, held := i.records[id]; held {
		return out.offer(op{row: RowOf(record)})
	}
	change, err := diskOp(i.root, id)
	if err != nil {
		return true
	}
	return out.offer(change)
}
