package tasks

import (
	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/workspace"
)

// RowOf is the row the index keeps for a task.
func RowOf(r Record) taskindex.Row {
	return taskindex.Row{ID: r.ID, State: string(r.State), Brief: r.Brief, Repo: r.Repo, AgentType: r.Harness, Model: r.Model,
		Instance: r.Instance, Thread: r.Thread, Asked: r.Asked, RequestKey: r.Key, StoppedBy: r.StoppedBy, Team: r.Team,
		Agent: r.Agent, HandedBy: r.HandedBy, Answers: r.Answers, Hops: r.Hops, PaidBy: r.PaidBy, Workspace: workspace.Of(r.Workspace), Seq: r.Seq, Exit: r.Exit, Queued: r.Queued, Started: r.Started, Ended: r.Ended}
}

// RecordOf is the task a row describes: everything in the record that a row holds.
func RecordOf(r taskindex.Row) Record {
	return Record{ID: r.ID, State: State(r.State), Brief: r.Brief, Repo: r.Repo, Harness: r.AgentType, Model: r.Model,
		Instance: r.Instance, Thread: r.Thread, Asked: r.Asked, Key: r.RequestKey, StoppedBy: r.StoppedBy, Team: r.Team,
		Agent: r.Agent, HandedBy: r.HandedBy, Answers: r.Answers, Hops: r.Hops, PaidBy: r.PaidBy, Workspace: workspace.Stored(r.Workspace), Seq: r.Seq, Exit: r.Exit, Queued: r.Queued, Started: r.Started, Ended: r.Ended}
}
