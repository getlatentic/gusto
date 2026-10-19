package usage

import (
	"database/sql"
	"strconv"
	"strings"
	"time"
)

// dialect holds every place the two databases differ; the queries are otherwise shared.
type dialect struct {
	name string
	// rebind turns the ? placeholders of a query into the form the driver wants.
	rebind func(string) string
	// timeArg is how a time is bound: SQLite keeps nanoseconds since the epoch as an integer, and
	// Postgres a timestamptz.
	timeArg func(time.Time) any
	// readOnly opens a snapshot for a read that takes several queries.
	readOnly *sql.TxOptions
	// contains is true of a row whose search column holds the one argument.
	contains string
	// filteredGroup is how sessions are grouped when a filter picks the rows. SQLite would otherwise walk the
	// session index and fetch every row from the table to test it, where a scan of the table is far quicker.
	filteredGroup string
	// hasMigrations is a query that is 1 when the schema_migrations table exists and 0 when it does not.
	hasMigrations string
	// lock, when set, serialises migrations of one database across processes.
	lock string
}

var sqliteDialect = dialect{
	name:          "sqlite",
	rebind:        func(q string) string { return q },
	timeArg:       func(t time.Time) any { return t.UnixNano() },
	contains:      "instr(search, ?) > 0",
	filteredGroup: "+session",
	hasMigrations: `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
}

var postgresDialect = dialect{
	name:          "postgres",
	rebind:        dollarPlaceholders,
	timeArg:       func(t time.Time) any { return t.UTC() },
	contains:      "strpos(search, ?) > 0",
	filteredGroup: "session",
	readOnly:      &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead},
	hasMigrations: `SELECT CASE WHEN to_regclass('schema_migrations') IS NULL THEN 0 ELSE 1 END`,
	lock:          `SELECT pg_advisory_xact_lock(7243013299)`,
}

func dollarPlaceholders(q string) string {
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// timeCol reads a time column in either dialect's form.
type timeCol struct{ time.Time }

func (c *timeCol) Scan(v any) error {
	switch x := v.(type) {
	case int64:
		c.Time = time.Unix(0, x).UTC()
	case time.Time:
		c.Time = x.UTC()
	default:
		return errTimeColumn
	}
	return nil
}
