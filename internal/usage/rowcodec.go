package usage

import (
	"database/sql"
	"strings"
)

var rowColumns = []string{"task", "line", "called_at", "session", "person", "team", "project", "engine", "agent",
	"provider", "credential", "model", "input_tokens", "output_tokens", "cached_tokens", "usd", "status",
	"took_ms", "refused", "translated", "search"}

// selectColumns is every Row field, without the search column that only the database reads.
var selectColumns = strings.Join(rowColumns[:len(rowColumns)-1], ", ")

func (r Row) appendArgs(args []any, d dialect) []any {
	return append(args, r.Task, r.Line, d.timeArg(r.At), r.Session, r.Person, r.Team, r.Project, r.Engine, r.Agent,
		r.Provider, r.Credential, r.Model, r.InputTokens, r.OutputTokens, r.CachedTokens, r.USD, r.Status,
		r.TookMS, r.Refused, r.Translated, r.searchText())
}

func scanRow(rows *sql.Rows) (Row, error) {
	var r Row
	var at timeCol
	var in, out, cached sql.NullInt64
	var usd sql.NullFloat64
	err := rows.Scan(&r.Task, &r.Line, &at, &r.Session, &r.Person, &r.Team, &r.Project, &r.Engine, &r.Agent,
		&r.Provider, &r.Credential, &r.Model, &in, &out, &cached, &usd, &r.Status, &r.TookMS, &r.Refused, &r.Translated)
	if err != nil {
		return Row{}, err
	}
	r.At = at.Time
	r.InputTokens, r.OutputTokens, r.CachedTokens = nullInt(in), nullInt(out), nullInt(cached)
	if usd.Valid {
		r.USD = &usd.Float64
	}
	return r, nil
}

func nullInt(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}
