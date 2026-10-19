package jobs

import "github.com/getlatentic/gusto/internal/taskindex"

// RowOf is the row the index keeps for a task.
func RowOf(r Record) taskindex.Row {
	return taskindex.Row{ID: r.ID, State: string(r.State), Brief: r.Brief, Repo: r.Repo, AgentType: r.Harness, Model: r.Model,
		Instance: r.Instance, Thread: r.Thread, Asked: r.Asked, RequestKey: r.Key, StoppedBy: r.StoppedBy, Team: r.Team,
		Seq: r.Seq, Exit: r.Exit, Queued: r.Queued, Started: r.Started, Ended: r.Ended}
}

// RecordOf is the task a row describes: everything in the record that a row holds.
func RecordOf(r taskindex.Row) Record {
	return Record{ID: r.ID, State: State(r.State), Brief: r.Brief, Repo: r.Repo, Harness: r.AgentType, Model: r.Model,
		Instance: r.Instance, Thread: r.Thread, Asked: r.Asked, Key: r.RequestKey, StoppedBy: r.StoppedBy, Team: r.Team,
		Seq: r.Seq, Exit: r.Exit, Queued: r.Queued, Started: r.Started, Ended: r.Ended}
}
