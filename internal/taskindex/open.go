package taskindex

import (
	"context"
	"embed"

	"github.com/getlatentic/gusto/internal/dbkit"
)

const databaseFile = "tasks.db"

//go:embed migrations
var migrationFiles embed.FS

var migrations = dbkit.Migrations{Scope: "tasks", Table: "tasks_schema_migrations", FS: migrationFiles, Dir: "migrations"}

// Open opens the index and brings its schema up to date. An empty databaseURL is SQLite in the file tasks.db
// in dir; a postgres:// or postgresql:// URL is Postgres, whose search_path parameter names the schema to keep
// the tables in; a sqlite:///path URL is SQLite in another file. No error it returns carries the URL's password.
func Open(ctx context.Context, databaseURL, dir string) (Store, error) {
	db, err := dbkit.Open(ctx, databaseURL, dir, databaseFile)
	if err != nil {
		return nil, err
	}
	if err := dbkit.Migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return &store{db: db, contains: containsOf(db.Dialect())}, nil
}

// Version reports the schema version a database is at and the newest this program knows, without changing
// the database. A database that does not exist yet is at 0.
func Version(ctx context.Context, databaseURL, dir string) (current, latest int, err error) {
	return dbkit.Version(ctx, databaseURL, dir, databaseFile, migrations)
}

func containsOf(d dbkit.Dialect) string {
	if d == dbkit.Postgres {
		return "strpos(search, ?) > 0"
	}
	return "instr(search, ?) > 0"
}
