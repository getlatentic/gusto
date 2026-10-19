package dbkit

import "strings"

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ContainsPattern is the argument that makes Like match a column that holds s as a substring, with any %,
// _ or \ in s taken as itself.
func ContainsPattern(s string) string { return "%" + likeEscaper.Replace(s) + "%" }

// Like is a condition with one ? that is true of a column whose text matches the pattern, without regard
// to case. Postgres folds all letters; SQLite folds only ASCII ones, so for other letters keep a lower-cased
// copy of the text in its own column and compare that.
func (db *DB) Like(column string) string {
	if db.dialect == Postgres {
		return column + ` ILIKE ? ESCAPE '\'`
	}
	return column + ` LIKE ? ESCAPE '\'`
}
