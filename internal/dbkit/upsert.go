package dbkit

import "strings"

// Upsert describes an INSERT of one or more rows that, when a row's Conflict columns are already in the
// table, either leaves the old row alone (Update empty) or sets the Update columns from the new row. Both
// databases take this syntax.
type Upsert struct {
	Table    string
	Columns  []string
	Conflict []string
	Update   []string
}

// SQL is the statement for rows rows, with one ? for each value, row after row in the order of Columns.
func (u Upsert) SQL(rows int) string {
	group := "(" + strings.TrimSuffix(strings.Repeat("?,", len(u.Columns)), ",") + ")"
	var b strings.Builder
	b.WriteString("INSERT INTO " + u.Table + " (" + strings.Join(u.Columns, ", ") + ") VALUES ")
	b.WriteString(strings.TrimSuffix(strings.Repeat(group+",", rows), ","))
	b.WriteString(" ON CONFLICT (" + strings.Join(u.Conflict, ", ") + ") ")
	if len(u.Update) == 0 {
		b.WriteString("DO NOTHING")
		return b.String()
	}
	sets := make([]string, len(u.Update))
	for i, c := range u.Update {
		sets[i] = c + " = excluded." + c
	}
	b.WriteString("DO UPDATE SET " + strings.Join(sets, ", "))
	return b.String()
}
