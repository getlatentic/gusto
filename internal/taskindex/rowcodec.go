package taskindex

import "strings"

// rowColumns are the table's columns, with the ones a Row holds first and the ones that only the database reads after them.
var rowColumns = []string{"id", "state", "brief", "repo", "harness", "model", "instance", "thread", "asked", "request_key",
	"stopped_by", "team", "workspace", "agent", "handed_by", "answers", "hops", "paid_by", "seq", "exit_code", "queued",
	"started", "ended", "asked_lc", "search", "sum", "progress"}

// rowFields is how many of rowColumns a Row holds.
const rowFields = 23

var selectColumns = strings.Join(rowColumns[:rowFields], ", ")

func (r Row) appendArgs(args []any) []any {
	return append(args, r.ID, r.State, r.Brief, r.Repo, r.AgentType, r.Model, r.Instance, r.Thread, r.Asked, r.RequestKey,
		r.StoppedBy, r.Team, r.Workspace, r.Agent, r.HandedBy, r.Answers, r.Hops, r.PaidBy, r.Seq, r.Exit, r.Queued, r.Started,
		r.Ended, strings.ToLower(r.Asked), r.SearchText(), r.Sum(), r.Progress())
}

func scanRow(rows interface{ Scan(...any) error }) (Row, error) {
	var r Row
	err := rows.Scan(&r.ID, &r.State, &r.Brief, &r.Repo, &r.AgentType, &r.Model, &r.Instance, &r.Thread, &r.Asked,
		&r.RequestKey, &r.StoppedBy, &r.Team, &r.Workspace, &r.Agent, &r.HandedBy, &r.Answers, &r.Hops, &r.PaidBy, &r.Seq,
		&r.Exit, &r.Queued, &r.Started, &r.Ended)
	return r, err
}
