package roomwork

import "context"

// Merge brings what the turn changed into the room's folder. A file only the turn changed is put there as it is. A file another
// turn or a person changed too keeps the version already in the folder, and the turn's is saved beside it: the Outcome names
// each one. Either every change reaches the folder or none does, and the turn's copy is not touched.
//
// Merge is for a turn that passed. A turn that failed or was stopped is closed without it.
func (t *Turn) Merge(ctx context.Context) (Outcome, error) {
	if t.closed.Load() {
		return Outcome{}, ErrClosed
	}
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
	actions, conflicts := t.plan(changes, now)
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
	for _, each := range conflicts {
		if each.Side != "" {
			saved++
		}
	}
	return Outcome{Changed: len(actions) - saved, Conflicts: conflicts}, nil
}
