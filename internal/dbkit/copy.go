package dbkit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrNotEmpty is a copy into a table that already has rows, which a copy would mix with what it brings.
var ErrNotEmpty = errors.New("dbkit: the table to copy into already has rows")

// Tables is the tables of a SQLite database, but for the ones named in except and SQLite's own, in an order a copy can
// write them in: a table comes after every table its foreign keys point at.
func Tables(ctx context.Context, db *DB, except ...string) ([]string, error) {
	if db.dialect != SQLite {
		return nil, fmt.Errorf("dbkit: tables are listed from a SQLite database, not %s", db.dialect)
	}
	names, err := column(ctx, db, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite\\_%' ESCAPE '\\' ORDER BY name")
	if err != nil {
		return nil, err
	}
	refers := map[string][]string{}
	for _, name := range names {
		if refers[name], err = column(ctx, db, "SELECT DISTINCT \"table\" FROM pragma_foreign_key_list(?)", name); err != nil {
			return nil, err
		}
	}
	return referencedFirst(names, refers, except)
}

// referencedFirst orders the tables so that each comes after the tables it refers to, leaving out those in except.
func referencedFirst(names []string, refers map[string][]string, except []string) ([]string, error) {
	var ordered []string
	placed, placing := map[string]bool{}, map[string]bool{}
	var place func(name string) error
	place = func(name string) error {
		if placed[name] {
			return nil
		}
		if placing[name] {
			return fmt.Errorf("dbkit: the foreign keys of %s go round in a circle", name)
		}
		placing[name] = true
		for _, other := range refers[name] {
			if other != name {
				if err := place(other); err != nil {
					return err
				}
			}
		}
		placed[name] = true
		if !slices.Contains(except, name) {
			ordered = append(ordered, name)
		}
		return nil
	}
	for _, name := range names {
		if err := place(name); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// column is the one column of text a query answers with, row by row.
func column(ctx context.Context, db *DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// Copy copies every row of the tables named from a SQLite database into a Postgres one whose schema is the same, in one
// transaction, and says how many rows each table had. A time, which SQLite keeps as nanoseconds since the epoch, and a
// boolean, which it keeps as 0 or 1, are given to Postgres as the types its columns are. A table that has rows already is
// not copied into, and nothing is.
func Copy(ctx context.Context, from, to *DB, tables []string) (map[string]int, error) {
	if from.dialect != SQLite || to.dialect != Postgres {
		return nil, fmt.Errorf("dbkit: a copy is from SQLite into Postgres, not from %s into %s", from.dialect, to.dialect)
	}
	copied := map[string]int{}
	err := to.InTx(ctx, func(t Tx) error {
		for _, table := range tables {
			n, err := copyTable(ctx, from, t, table)
			if err != nil {
				return fmt.Errorf("%s: %w", table, err)
			}
			copied[table] = n
		}
		return nil
	})
	return copied, err
}

func copyTable(ctx context.Context, from *DB, t Tx, table string) (int, error) {
	var held int
	if err := t.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoted(table)).Scan(&held); err != nil {
		return 0, err
	}
	if held > 0 {
		return 0, ErrNotEmpty
	}
	kinds, err := columnKinds(ctx, t, table)
	if err != nil {
		return 0, err
	}
	rows, err := from.QueryContext(ctx, "SELECT * FROM "+quoted(table))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	return insertRows(ctx, t, table, rows, kinds)
}

// insertRows inserts every row a query of a SQLite table answers into the Postgres table of that name, and says how many.
func insertRows(ctx context.Context, t Tx, table string, rows *sql.Rows, kinds map[string]string) (int, error) {
	columns, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	insert := insertInto(table, columns)
	n := 0
	for rows.Next() {
		values, err := scanned(rows, len(columns))
		if err != nil {
			return n, err
		}
		for i, column := range columns {
			if values[i], err = asPostgres(values[i], kinds[column]); err != nil {
				return n, fmt.Errorf("%s: %w", column, err)
			}
		}
		if _, err := t.ExecContext(ctx, insert, values...); err != nil {
			return n, err
		}
		n++
	}
	return n, rows.Err()
}

// columnKinds is each column of a Postgres table by the type information_schema names it with.
func columnKinds(ctx context.Context, t Tx, table string) (map[string]string, error) {
	rows, err := t.QueryContext(ctx, "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ?", table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	kinds := map[string]string{}
	for rows.Next() {
		var column, kind string
		if err := rows.Scan(&column, &kind); err != nil {
			return nil, err
		}
		kinds[column] = kind
	}
	if len(kinds) == 0 && rows.Err() == nil {
		return nil, fmt.Errorf("the database copied into has no table %s", table)
	}
	return kinds, rows.Err()
}

func scanned(rows *sql.Rows, n int) ([]any, error) {
	values := make([]any, n)
	into := make([]any, n)
	for i := range values {
		into[i] = &values[i]
	}
	return values, rows.Scan(into...)
}

// asPostgres is a value SQLite held, as the Postgres column of that kind takes it.
func asPostgres(value any, kind string) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch kind {
	case "timestamp with time zone":
		nanos, ok := value.(int64)
		if !ok {
			return nil, ErrTimeColumn
		}
		return time.Unix(0, nanos).UTC(), nil
	case "boolean":
		number, ok := value.(int64)
		if !ok {
			return nil, fmt.Errorf("a boolean column held %T", value)
		}
		return number != 0, nil
	}
	if bytes, ok := value.([]byte); ok && kind != "bytea" {
		return string(bytes), nil
	}
	return value, nil
}

func insertInto(table string, columns []string) string {
	names := make([]string, len(columns))
	marks := make([]string, len(columns))
	for i, column := range columns {
		names[i], marks[i] = quoted(column), "?"
	}
	return "INSERT INTO " + quoted(table) + " (" + strings.Join(names, ", ") + ") VALUES (" + strings.Join(marks, ", ") + ")"
}

// quoted is a name as an identifier in either database. The names come from the database's own catalogue.
func quoted(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
