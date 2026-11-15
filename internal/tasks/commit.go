package tasks

import (
	"errors"
	"fmt"
)

// ErrStoppedFirst is the answer to committing a task that somebody stopped before its work became final.
var ErrStoppedFirst = errors.New("that task was stopped before its work was kept")

// errKept is the answer to stopping a task whose work is already final.
var errKept = fmt.Errorf("%w: its work is already kept", ErrNotStoppable)

// Commit is called by a running task just before it does what cannot be taken back, such as merging its changes into a room's
// folder. From then on the task cannot be stopped, since a stop would end it interrupted after its work was kept, and the
// record would say two things. A task stopped before this is not committed: it must not do the work, and ends interrupted.
func (i *Index) Commit(id string) error {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	switch {
	case !found:
		return fmt.Errorf("no task called %s", id)
	case record.State.Done():
		return ErrEnded
	case record.StoppedBy != "":
		return ErrStoppedFirst
	}
	i.committed[id] = true
	return nil
}
