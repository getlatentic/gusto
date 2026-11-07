package taskindex

import "strings"

var rowColumns = []string{"id", "state", "brief", "repo", "harness", "model", "instance", "thread", "asked", "asked_lc",
	"request_key", "stopped_by", "team", "workspace", "seq", "exit_code", "queued", "started", "ended", "search", "sum"}

// selectColumns is every Row field, without the columns that only the database reads.
var selectColumns = strings.Join(append(rowColumns[:9:9], rowColumns[10:19]...), ", ")

func (r Row) appendArgs(args []any) []any {
	return append(args, r.ID, r.State, r.Brief, r.Repo, r.AgentType, r.Model, r.Instance, r.Thread, r.Asked,
		strings.ToLower(r.Asked), r.RequestKey, r.StoppedBy, r.Team, r.Workspace, r.Seq, r.Exit, r.Queued, r.Started, r.Ended,
		r.SearchText(), r.Sum())
}

func scanRow(rows interface{ Scan(...any) error }) (Row, error) {
	var r Row
	err := rows.Scan(&r.ID, &r.State, &r.Brief, &r.Repo, &r.AgentType, &r.Model, &r.Instance, &r.Thread, &r.Asked,
		&r.RequestKey, &r.StoppedBy, &r.Team, &r.Workspace, &r.Seq, &r.Exit, &r.Queued, &r.Started, &r.Ended)
	return r, err
}
