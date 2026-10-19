package dbkit

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const sqlitePragmas = "_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"

// Open opens a database and, on Postgres with a search_path, creates that schema when it is missing. An
// empty databaseURL is the SQLite file named file in dir; a postgres:// or postgresql:// URL is Postgres,
// whose search_path parameter names the schema the tables live in; a sqlite:///path URL is SQLite in another
// file. Nothing it returns carries a password.
func Open(ctx context.Context, databaseURL, dir, file string) (*DB, error) {
	t, err := parseTarget(databaseURL, dir, file)
	if err != nil {
		return nil, err
	}
	if t.dialect == SQLite {
		if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
			return nil, fmt.Errorf("dbkit: make the data directory: %w", err)
		}
	}
	db, err := connect(ctx, t)
	if err != nil {
		return nil, err
	}
	if err := db.ensureSchema(ctx); err != nil {
		db.Close()
		return nil, t.redact(err)
	}
	return db, nil
}

func connect(ctx context.Context, t target) (*DB, error) {
	if t.dialect == Postgres {
		return connectPostgres(ctx, t)
	}
	return connectSQLite(ctx, t)
}

func connectPostgres(ctx context.Context, t target) (*DB, error) {
	pool := stdlib.OpenDB(*t.pg)
	pool.SetMaxOpenConns(16)
	pool.SetConnMaxIdleTime(5 * time.Minute)
	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, t.redact(fmt.Errorf("dbkit: cannot reach the database: %w", err))
	}
	return &DB{dialect: Postgres, w: pool, r: pool, target: t}, nil
}

func connectSQLite(ctx context.Context, t target) (*DB, error) {
	w, err := sql.Open("sqlite", t.path+"?"+sqlitePragmas+"&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("dbkit: open %s: %w", t.path, err)
	}
	w.SetMaxOpenConns(1)
	if err := w.PingContext(ctx); err != nil {
		w.Close()
		return nil, fmt.Errorf("dbkit: open %s: %w", t.path, err)
	}
	r, err := sql.Open("sqlite", t.path+"?"+sqlitePragmas+"&_pragma=query_only(1)")
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("dbkit: open %s: %w", t.path, err)
	}
	r.SetMaxOpenConns(max(4, runtime.NumCPU()))
	return &DB{dialect: SQLite, w: w, r: r, target: t}, nil
}

// ensureSchema creates the Postgres schema the URL names, under a lock so that two processes starting
// together do not collide on it.
func (db *DB) ensureSchema(ctx context.Context) error {
	if db.target.schema == "" {
		return nil
	}
	return db.InTx(ctx, func(tx Tx) error {
		if err := db.lock(ctx, tx, "schema:"+db.target.schema); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS "`+db.target.schema+`"`)
		if err != nil {
			return fmt.Errorf("dbkit: create schema %s: %w", db.target.schema, err)
		}
		return nil
	})
}
