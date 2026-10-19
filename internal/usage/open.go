package usage

import (
	"context"
	"embed"

	"github.com/getlatentic/gusto/internal/dbkit"
)

const databaseFile = "usage.db"

//go:embed migrations
var migrationFiles embed.FS

// migrations keeps its version table under the name schema_migrations, which is where databases made
// before dbkit hold it.
var migrations = dbkit.Migrations{Scope: "usage", Table: "schema_migrations", FS: migrationFiles, Dir: "migrations"}

// Open opens the index and brings its schema up to date. An empty databaseURL is SQLite in dir; a
// postgres:// or postgresql:// URL is Postgres, whose search_path parameter names the schema to keep the
// tables in; a sqlite:///path URL is SQLite in another file.
func Open(ctx context.Context, databaseURL, dir string) (Store, error) {
	db, err := dbkit.Open(ctx, databaseURL, dir, databaseFile)
	if err != nil {
		return nil, err
	}
	if err := dbkit.Migrate(ctx, db, migrations); err != nil {
		db.Close()
		return nil, err
	}
	return newStore(db), nil
}

// Version reports the schema version a database is at and the newest this program knows, without changing
// the database. A database that does not exist yet is at 0.
func Version(ctx context.Context, databaseURL, dir string) (current, latest int, err error) {
	return dbkit.Version(ctx, databaseURL, dir, databaseFile, migrations)
}
