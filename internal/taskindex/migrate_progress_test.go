package taskindex_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
	"github.com/getlatentic/gusto/internal/taskindex"
)

// oldIndex makes a database as it was at schema version 2, with a row in each state, two with a spending label as their team.
func oldIndex(t *testing.T, driver, dialect, url string) {
	t.Helper()
	db, err := sql.Open(driver, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if dialect == "postgres" {
		// Opening makes the schema; the tables it made are replaced with the old ones.
		s, err := taskindex.Open(ctx, url, "")
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
		if _, err := db.Exec(`DROP TABLE tasks; DROP TABLE tasks_schema_migrations`); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"0001_tasks.sql", "0002_workspace.sql"} {
		body, err := os.ReadFile(filepath.Join("migrations", dialect, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	insert := `INSERT INTO tasks (id, state, stopped_by, team, workspace, seq, queued, search, sum) VALUES `
	if _, err := db.Exec(insert + `('q', 'queued', '', '', 'main', 1, 1, 'q', 's'), ('r', 'running', '', '', 'main', 2, 2, 'r', 's'),
		('s', 'running', 'bob', 'platform', 'main', 3, 3, 's', 's'), ('p', 'passed', '', 'platform', 'eng', 4, 4, 'p', 's'),
		('i', 'interrupted', 'bob', '', 'main', 5, 5, 'i', 's')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE tasks_schema_migrations (version BIGINT PRIMARY KEY, name TEXT NOT NULL, applied_at BIGINT NOT NULL);
		INSERT INTO tasks_schema_migrations VALUES (1, '0001_tasks.sql', 0), (2, '0002_workspace.sql', 0)`); err != nil {
		t.Fatal(err)
	}
}

func progressAfterMigration(t *testing.T, url, dir string) {
	t.Helper()
	s, err := taskindex.Open(ctx, url, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for id, want := range map[string]int{"q": 0, "r": 1, "s": 2, "p": 3, "i": 3} {
		row, err := s.Get(ctx, id)
		if err != nil || row.Progress() != want || row.Agent != "" || row.Hops != 0 {
			t.Fatalf("%s: %+v %v", id, row, err)
		}
	}
	// A task's team is the one the workspace column held, and the label once kept as its team is gone.
	for id, want := range map[string]string{"s": "main", "p": "eng"} {
		if row, _ := s.Get(ctx, id); row.Team != want {
			t.Fatalf("%s is in team %q, not %q", id, row.Team, want)
		}
	}
	// A row of the old index is behind a newer one, so a writer that is late with an old state cannot undo it.
	old, _ := s.Get(ctx, "r")
	old.State = "passed"
	put(t, s, old)
	put(t, s, with(old, func(r *taskindex.Row) { r.State = "queued" }))
	if got, _ := s.Get(ctx, "r"); got.State != "passed" {
		t.Fatalf("got %+v", got)
	}
}

func TestMigrationKeepsOldRowsInProgressOrder(t *testing.T) {
	dir := t.TempDir()
	oldIndex(t, "sqlite", "sqlite", "file:"+sqlitePath(dir))
	progressAfterMigration(t, "", dir)
}

func TestMigrationKeepsOldPostgresRowsInProgressOrder(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	url := dbkittest.PostgresURL(t)
	oldIndex(t, "pgx", "postgres", url)
	progressAfterMigration(t, url, "")
}
