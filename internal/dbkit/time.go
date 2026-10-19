package dbkit

import (
	"errors"
	"time"
)

// ErrTimeColumn is a time column that held a value of the wrong kind.
var ErrTimeColumn = errors.New("dbkit: a time column held a value of the wrong kind")

// Times are stored in UTC. A Postgres column is timestamptz. A SQLite column is INTEGER, holding
// nanoseconds since the Unix epoch, so that it sorts and compares as a number and keeps every digit a
// time.Time has. Postgres keeps microseconds; Now returns a time cut to that, so that what a program
// writes is what it reads back from either database.

// Now is the current time in UTC, at the precision both databases keep.
func Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// TimeArg is how a time is bound as a query argument for this database's time columns.
func (db *DB) TimeArg(t time.Time) any {
	if db.dialect == SQLite {
		return t.UnixNano()
	}
	return t.UTC()
}

// Time reads a time column in either database's form. Scan it into a sql.Null[Time] when the column can
// be NULL.
type Time struct{ time.Time }

// Scan implements sql.Scanner.
func (c *Time) Scan(v any) error {
	switch x := v.(type) {
	case int64:
		c.Time = time.Unix(0, x).UTC()
	case time.Time:
		c.Time = x.UTC()
	default:
		return ErrTimeColumn
	}
	return nil
}
