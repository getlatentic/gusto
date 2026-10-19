package dbkit

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// Dialect names the database behind a DB.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// Querier is what a DB and a Tx share. Every query is written with ? placeholders.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// DB is an open database. For SQLite, writes go through one connection that takes the write lock when a
// transaction starts, and reads go through a pool of query-only connections; for Postgres both are the same
// pool.
type DB struct {
	dialect Dialect
	w, r    *sql.DB
	target  target
}

var _ Querier = (*DB)(nil)

// Dialect is the database this DB talks to.
func (db *DB) Dialect() Dialect { return db.dialect }

// Reader is the pool for reads. It is for sql.Rows-level work; plain queries can go through the DB itself.
func (db *DB) Reader() *sql.DB { return db.r }

// Writer is the pool for writes: one connection for SQLite.
func (db *DB) Writer() *sql.DB { return db.w }

// URLForLog is the URL the DB was opened with, minus any password, safe to print.
func (db *DB) URLForLog() string { return db.target.logURL }

// Close closes every pool.
func (db *DB) Close() error {
	err := db.w.Close()
	if db.r != db.w {
		err = errors.Join(err, db.r.Close())
	}
	return err
}

// QueryContext runs a read.
func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.r.QueryContext(ctx, db.Rebind(query), args...)
}

// QueryRowContext runs a read of one row.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.r.QueryRowContext(ctx, db.Rebind(query), args...)
}

// ExecContext runs a write outside a transaction.
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.w.ExecContext(ctx, db.Rebind(query), args...)
}

// Rebind turns the ? placeholders of a query into the form the database wants: $1, $2 and so on for
// Postgres, unchanged for SQLite. A ? inside a quoted string is not a placeholder.
func (db *DB) Rebind(query string) string {
	if db.dialect != Postgres {
		return query
	}
	return dollarPlaceholders(query)
}

func dollarPlaceholders(q string) string {
	var b strings.Builder
	n, quote := 0, rune(0)
	for _, r := range q {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '?':
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
