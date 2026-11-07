package roomwork

import (
	"context"
	"errors"
)

// ErrMerged is the answer to merging a turn whose changes are in the folder already: a second merge would meet them as another
// turn's and conflict with itself.
var ErrMerged = errors.New("the turn has been merged")

// Merge brings what the turn changed into the room's folder. A file only the turn changed is put there as it is. A file another
// turn or a person changed too keeps the version already in the folder, and the turn's is saved beside it: the Outcome names
// each one. Either every change reaches the folder or none does, and the turn's copy is not touched.
//
// Merge is for a turn that passed. A turn that failed or was stopped is closed without it.
func (t *Turn) Merge(ctx context.Context) (Outcome, error) {
	t.merging.Lock()
	defer t.merging.Unlock()
	if t.closed.Load() {
		return Outcome{}, ErrClosed
	}
	if t.merged {
		return Outcome{}, ErrMerged
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(t.stopped, cancel)()
	outcome, err := t.merge(ctx)
	t.merged = err == nil
	return outcome, err
}

func (t *Turn) merge(ctx context.Context) (Outcome, error) {
	now, err := scan(t.dest)
	if err != nil {
		return Outcome{}, err
	}
	changes := t.changes(now)
	if len(changes) == 0 {
		return Outcome{}, nil
	}
	release, err := t.hub.hold(ctx, t.room)
	if err != nil {
		return Outcome{}, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	plan := t.plan(changes, now)
	actions := plan.actions
	steps, err := commit(t.room, actions)
	if err != nil {
		return Outcome{}, err
	}
	tidy(t.room.Folder, actions)
	paths := make([]string, len(actions))
	for i, each := range actions {
		paths[i] = each.dest
	}
	t.hub.merged(t.room, t, paths, steps)
	forget(t.room)
	saved := 0
	for _, each := range plan.conflicts {
		if each.Side != "" {
			saved++
		}
	}
	return Outcome{Changed: len(actions) - saved, Conflicts: plan.conflicts, Joined: plan.joined}, nil
}
