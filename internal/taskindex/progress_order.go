package taskindex

// Progress is how far along the task is, which only grows: queued, running, running and asked to stop, then over. It is kept in
// the progress column, so that one statement can tell whether a row is behind the one the index holds.
func (r Row) Progress() int {
	switch r.State {
	case "queued":
		return 0
	case "running":
		if r.StoppedBy != "" {
			return 2
		}
		return 1
	}
	return 3
}

// Replaces says whether this row may take the place of the held row of the same task. It may when it is further along, or as far
// along and ended no earlier. A row copied from an old read of the task's files is behind, so it cannot undo what a newer change
// wrote. The upsert in store.go says the same in SQL.
func (r Row) Replaces(held Row) bool { return r.ReplacesEntry(held.Entry()) }

// ReplacesEntry is Replaces for the part of a held row that Indexed returns.
func (r Row) ReplacesEntry(held Entry) bool {
	if r.Progress() != held.Progress {
		return r.Progress() > held.Progress
	}
	return r.Ended >= held.Ended
}
