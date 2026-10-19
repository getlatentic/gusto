package dbkit_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

func migrationFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func widgets(extra map[string]string) dbkit.Migrations {
	files := map[string]string{
		"m/sqlite/0001_widgets.sql":   `CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
		"m/postgres/0001_widgets.sql": `CREATE TABLE widgets (id BIGINT PRIMARY KEY, name TEXT NOT NULL);`,
		"m/sqlite/0002_index.sql":     `CREATE INDEX widgets_name ON widgets (name);`,
		"m/postgres/0002_index.sql":   `CREATE INDEX widgets_name ON widgets (name);`,
	}
	for k, v := range extra {
		files[k] = v
	}
	return dbkit.Migrations{Scope: "widgets", Table: "widget_migrations", FS: migrationFS(files), Dir: "m"}
}

func versionOf(t *testing.T, db *dbkit.DB, table string) (n int) {
	t.Helper()
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrateAppliesInOrderOnceAndAgain(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		m := widgets(nil)
		for range 2 {
			if err := dbkit.Migrate(ctx, db, m); err != nil {
				t.Fatal(err)
			}
		}
		if got := versionOf(t, db, "widget_migrations"); got != 2 {
			t.Fatalf("version = %d", got)
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM widget_migrations`).Scan(&n); err != nil || n != 2 {
			t.Fatalf("rows = %d %v", n, err)
		}
	})
}

func TestMigrateKeepsComponentsApart(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		other := dbkit.Migrations{Scope: "gadgets", Table: "gadget_migrations", Dir: "g", FS: migrationFS(map[string]string{
			"g/sqlite/0001_gadgets.sql":   `CREATE TABLE gadgets (id INTEGER PRIMARY KEY);`,
			"g/postgres/0001_gadgets.sql": `CREATE TABLE gadgets (id BIGINT PRIMARY KEY);`,
		})}
		if err := dbkit.Migrate(ctx, db, widgets(nil)); err != nil {
			t.Fatal(err)
		}
		if err := dbkit.Migrate(ctx, db, other); err != nil {
			t.Fatal(err)
		}
		if versionOf(t, db, "widget_migrations") != 2 || versionOf(t, db, "gadget_migrations") != 1 {
			t.Fatal("the version tables are not separate")
		}
	})
}

func TestMigrateRollsBackAFailedMigrationAndKeepsTheEarlierOnes(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		bad := widgets(map[string]string{
			"m/sqlite/0003_bad.sql":   `CREATE TABLE half (id INTEGER); SELECT * FROM nowhere;`,
			"m/postgres/0003_bad.sql": `CREATE TABLE half (id INTEGER); SELECT * FROM nowhere;`,
		})
		err := dbkit.Migrate(ctx, db, bad)
		if err == nil || !strings.Contains(err.Error(), "0003_bad.sql") {
			t.Fatalf("err = %v", err)
		}
		if got := versionOf(t, db, "widget_migrations"); got != 2 {
			t.Fatalf("version after failure = %d", got)
		}
		if _, err := db.ExecContext(ctx, `SELECT * FROM half`); err == nil {
			t.Fatal("the failed migration left a table behind")
		}
	})
}

func TestMigrateRefusesANewerDatabase(t *testing.T) {
	dbkittest.Each(t, func(t *testing.T, db *dbkit.DB) {
		m := widgets(nil)
		if err := dbkit.Migrate(ctx, db, m); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO widget_migrations (version, name, applied_at) VALUES (99, 'future.sql', 0)`); err != nil {
			t.Fatal(err)
		}
		err := dbkit.Migrate(ctx, db, m)
		if err == nil || !strings.Contains(err.Error(), "99") || !strings.Contains(err.Error(), "upgrade") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestMigrateRejectsBrokenMigrationSets(t *testing.T) {
	db := dbkittest.SQLite(t)
	for name, m := range map[string]dbkit.Migrations{
		"no files":        {Scope: "x", Table: "x_migrations", Dir: "none", FS: migrationFS(map[string]string{"none/sqlite/readme.txt": ""})},
		"no number":       {Scope: "x", Table: "x_migrations", Dir: "m", FS: migrationFS(map[string]string{"m/sqlite/first.sql": "SELECT 1;"})},
		"duplicate":       {Scope: "x", Table: "x_migrations", Dir: "m", FS: migrationFS(map[string]string{"m/sqlite/0001_a.sql": "SELECT 1;", "m/sqlite/0001_b.sql": "SELECT 1;"})},
		"unsafe table":    {Scope: "x", Table: "x; DROP TABLE y", Dir: "m", FS: migrationFS(map[string]string{"m/sqlite/0001_a.sql": "SELECT 1;"})},
		"missing dialect": {Scope: "x", Table: "x_migrations", Dir: "m", FS: migrationFS(map[string]string{"m/postgres/0001_a.sql": "SELECT 1;"})},
	} {
		if err := dbkit.Migrate(ctx, db, m); err == nil {
			t.Errorf("%s: migrated", name)
		}
	}
}

func TestVersionIsReadOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "yet")
	m := widgets(nil)
	cur, latest, err := dbkit.Version(ctx, "", dir, "w.db", m)
	if err != nil || cur != 0 || latest != 2 {
		t.Fatalf("version before open = %d %d %v", cur, latest, err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("Version made the data directory")
	}
	db, err := dbkit.Open(ctx, "", dir, "w.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if cur, _, err := dbkit.Version(ctx, "", dir, "w.db", m); err != nil || cur != 0 {
		t.Fatalf("version of a database with no version table = %d %v", cur, err)
	}
	if err := dbkit.Migrate(ctx, db, m); err != nil {
		t.Fatal(err)
	}
	if cur, latest, err := dbkit.Version(ctx, "", dir, "w.db", m); err != nil || cur != 2 || latest != 2 {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestPostgresVersionNeverCreatesTheSchema(t *testing.T) {
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		t.Skip(dbkittest.PostgresEnv + " is not set")
	}
	u := dbkittest.PostgresURL(t)
	m := widgets(nil)
	if cur, latest, err := dbkit.Version(ctx, u, "", "", m); err != nil || cur != 0 || latest != 2 {
		t.Fatalf("version of a schema not made yet = %d %d %v", cur, latest, err)
	}
	var found bool
	base, err := dbkit.Open(ctx, os.Getenv(dbkittest.PostgresEnv), "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	schema := u[strings.LastIndex(u, "=")+1:]
	if err := base.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = ?)`, schema).Scan(&found); err != nil || found {
		t.Fatalf("schema exists = %v %v", found, err)
	}
	db, err := dbkit.Open(ctx, u, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := dbkit.Migrate(ctx, db, m); err != nil {
		t.Fatal(err)
	}
	if cur, latest, err := dbkit.Version(ctx, u, "", "", m); err != nil || cur != latest {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestPostgresMigratedByManyProcessesAtOnce(t *testing.T) {
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		t.Skip(dbkittest.PostgresEnv + " is not set")
	}
	u := dbkittest.PostgresURL(t)
	m := widgets(nil)
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := dbkit.Open(ctx, u, "", "")
			if err != nil {
				errs[i] = err
				return
			}
			defer db.Close()
			errs[i] = dbkit.Migrate(ctx, db, m)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("process %d: %v", i, err)
		}
	}
	if cur, latest, err := dbkit.Version(ctx, u, "", "", m); err != nil || cur != latest {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}
