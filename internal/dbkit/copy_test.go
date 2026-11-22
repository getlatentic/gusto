package dbkit_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

// accounts is a schema with every kind of column the accounts database has: text, an integer, a boolean, a time that may
// be empty, and the migrations' own table, which a copy leaves out.
func accounts() dbkit.Migrations {
	return dbkit.Migrations{Scope: "copied", Table: "copied_schema_migrations", Dir: "m", FS: migrationFS(map[string]string{
		"m/sqlite/0001_people.sql": `CREATE TABLE people (email TEXT PRIMARY KEY, seats INTEGER NOT NULL, admin BOOLEAN NOT NULL,
			joined INTEGER NOT NULL, left_at INTEGER);`,
		"m/postgres/0001_people.sql": `CREATE TABLE people (email TEXT PRIMARY KEY, seats BIGINT NOT NULL, admin BOOLEAN NOT NULL,
			joined TIMESTAMPTZ NOT NULL, left_at TIMESTAMPTZ);`,
	})}
}

func migrated(t *testing.T, db *dbkit.DB) *dbkit.DB {
	t.Helper()
	if err := dbkit.Migrate(context.Background(), db, accounts()); err != nil {
		t.Fatal(err)
	}
	return db
}

func postgresOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		t.Skip(dbkittest.PostgresEnv + " is not set")
	}
}

func TestEveryRowOfASQLiteDatabaseIsCopiedIntoPostgresAsItsColumnsTakeIt(t *testing.T) {
	postgresOrSkip(t)
	ctx := context.Background()
	from, to := migrated(t, dbkittest.SQLite(t)), migrated(t, dbkittest.Postgres(t))
	joined := time.Date(2026, 11, 22, 7, 47, 41, 884122000, time.UTC)
	insert := "INSERT INTO people (email, seats, admin, joined, left_at) VALUES (?, ?, ?, ?, ?)"
	if _, err := from.ExecContext(ctx, insert, "ada@acme.com", 3, true, from.TimeArg(joined), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := from.ExecContext(ctx, insert, "bob@acme.com", 0, false, from.TimeArg(joined), from.TimeArg(joined.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	tables, err := dbkit.Tables(ctx, from, accounts().Table)
	if err != nil || len(tables) != 1 || tables[0] != "people" {
		t.Fatalf("the tables to copy are %v (%v), want people alone", tables, err)
	}
	copied, err := dbkit.Copy(ctx, from, to, tables)
	if err != nil || copied["people"] != 2 {
		t.Fatalf("copied %v (%v)", copied, err)
	}
	var seats int
	var admin bool
	var when dbkit.Time
	var left *time.Time
	row := to.QueryRowContext(ctx, "SELECT seats, admin, joined, left_at FROM people WHERE email = ?", "ada@acme.com")
	if err := row.Scan(&seats, &admin, &when, &left); err != nil {
		t.Fatal(err)
	}
	if seats != 3 || !admin || !when.Equal(joined) || left != nil {
		t.Errorf("ada came across as %d %t %s %v", seats, admin, when.Time, left)
	}
	if err := to.QueryRowContext(ctx, "SELECT admin, left_at FROM people WHERE email = ?", "bob@acme.com").Scan(&admin, &left); err != nil {
		t.Fatal(err)
	}
	if admin || left == nil || !left.Equal(joined.Add(time.Hour)) {
		t.Errorf("bob came across as %t %v", admin, left)
	}
}

func TestACopyIntoATableThatHasRowsCopiesNothing(t *testing.T) {
	postgresOrSkip(t)
	ctx := context.Background()
	from, to := migrated(t, dbkittest.SQLite(t)), migrated(t, dbkittest.Postgres(t))
	insert := "INSERT INTO people (email, seats, admin, joined) VALUES (?, ?, ?, ?)"
	from.ExecContext(ctx, insert, "ada@acme.com", 1, true, from.TimeArg(time.Now()))
	if _, err := to.ExecContext(ctx, insert, "there@acme.com", 1, false, to.TimeArg(time.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := dbkit.Copy(ctx, from, to, []string{"people"}); !errors.Is(err, dbkit.ErrNotEmpty) {
		t.Fatalf("copying over rows gave %v", err)
	}
	var n int
	to.QueryRowContext(ctx, "SELECT COUNT(*) FROM people").Scan(&n)
	if n != 1 {
		t.Errorf("the table has %d rows after a refused copy", n)
	}
}

func TestACopyIsFromSQLiteIntoPostgresOnly(t *testing.T) {
	a, b := dbkittest.SQLite(t), dbkittest.SQLite(t)
	if _, err := dbkit.Copy(context.Background(), a, b, nil); err == nil {
		t.Error("a copy from SQLite into SQLite went ahead")
	}
}

func TestTablesComeAfterTheTablesTheirForeignKeysPointAt(t *testing.T) {
	db := dbkittest.SQLite(t)
	schema := dbkit.Migrations{Scope: "ordered", Table: "ordered_schema_migrations", Dir: "m", FS: migrationFS(map[string]string{
		"m/sqlite/0001_t.sql": `CREATE TABLE memberships (person TEXT NOT NULL REFERENCES people(email), org TEXT NOT NULL REFERENCES orgs(slug));
			CREATE TABLE people (email TEXT PRIMARY KEY, invited_by TEXT REFERENCES people(email));
			CREATE TABLE orgs (slug TEXT PRIMARY KEY);`,
		"m/postgres/0001_t.sql": `SELECT 1;`,
	})}
	if err := dbkit.Migrate(context.Background(), db, schema); err != nil {
		t.Fatal(err)
	}
	tables, err := dbkit.Tables(context.Background(), db, schema.Table)
	if err != nil {
		t.Fatal(err)
	}
	at := map[string]int{}
	for i, name := range tables {
		at[name] = i
	}
	if len(tables) != 3 || at["memberships"] < at["orgs"] || at["memberships"] < at["people"] {
		t.Errorf("tables in the order %v: memberships must come after orgs and people", tables)
	}
}
