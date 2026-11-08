package dbkit_test

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

func TestAMigrationSetWithAGapIsRefused(t *testing.T) {
	db := dbkittest.SQLite(t)
	gap := dbkit.Migrations{Scope: "x", Table: "x_migrations", Dir: "m", FS: migrationFS(map[string]string{
		"m/sqlite/0001_a.sql": "CREATE TABLE a (id INTEGER);", "m/sqlite/0003_c.sql": "CREATE TABLE c (id INTEGER);",
	})}
	if err := dbkit.Migrate(ctx, db, gap); err == nil || !strings.Contains(err.Error(), "0003_c.sql") {
		t.Fatalf("a set with no 2 was migrated: %v", err)
	}
}

func TestADatabaseThatPassedOverAMigrationIsRefused(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		m := widgets(nil)
		if err := dbkit.Migrate(ctx, db, m); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM widget_migrations WHERE version = 1`); err != nil {
			t.Fatal(err)
		}
		if err := dbkit.Migrate(ctx, db, m); err == nil || !strings.Contains(err.Error(), "0001_widgets.sql") {
			t.Fatalf("a database that never had migration 1 was taken as up to date: %v", err)
		}
	})
}

func TestADatabaseThatHadAnotherMigrationUnderANumberIsRefused(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		if err := dbkit.Migrate(ctx, db, widgets(nil)); err != nil {
			t.Fatal(err)
		}
		renumbered := dbkit.Migrations{Scope: "widgets", Table: "widget_migrations", Dir: "m", FS: migrationFS(map[string]string{
			"m/sqlite/0001_widgets.sql":   `CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
			"m/postgres/0001_widgets.sql": `CREATE TABLE widgets (id BIGINT PRIMARY KEY, name TEXT NOT NULL);`,
			"m/sqlite/0002_colour.sql":    `ALTER TABLE widgets ADD COLUMN colour TEXT;`,
			"m/postgres/0002_colour.sql":  `ALTER TABLE widgets ADD COLUMN colour TEXT;`,
			"m/sqlite/0003_index.sql":     `CREATE INDEX widgets_name ON widgets (name);`,
			"m/postgres/0003_index.sql":   `CREATE INDEX widgets_name ON widgets (name);`,
		})}
		err := dbkit.Migrate(ctx, db, renumbered)
		if err == nil || !strings.Contains(err.Error(), "0002_index.sql") || !strings.Contains(err.Error(), "0002_colour.sql") {
			t.Fatalf("a database whose migration 2 was another was migrated on: %v", err)
		}
	})
}
