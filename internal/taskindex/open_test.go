package taskindex_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
	"github.com/getlatentic/gusto/internal/taskindex"
)

func TestOpenSQLiteInTheDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "yet")
	if cur, latest, err := taskindex.Version(ctx, "", dir); err != nil || cur != 0 || latest < 1 {
		t.Fatalf("version before open = %d %d %v", cur, latest, err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("Version made the data directory")
	}
	s, err := taskindex.Open(ctx, "", dir)
	if err != nil {
		t.Fatal(err)
	}
	put(t, s, task("a", 1))
	s.Close()
	if _, err := os.Stat(sqlitePath(dir)); err != nil {
		t.Fatal(err)
	}
	s, err = taskindex.Open(ctx, "", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, _ := s.Count(ctx, taskindex.Filter{}); got["queued"] != 1 {
		t.Fatalf("counts after reopen = %v", got)
	}
	if cur, latest, err := taskindex.Version(ctx, "", dir); err != nil || cur != latest || cur < 1 {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestOpenSQLiteURLNamesAnotherFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "elsewhere", "other.db")
	s, err := taskindex.Open(ctx, "sqlite://"+filepath.ToSlash(file), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRefusesUnknownSchemesAndListsTheForms(t *testing.T) {
	_, err := taskindex.Open(ctx, "mysql://root:hunter2@localhost/db", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "postgres://") || !strings.Contains(err.Error(), "tasks.db") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatal("the password is in the error")
	}
}

func TestOpenKeepsThePasswordOutOfErrors(t *testing.T) {
	for _, u := range []string{
		"postgres://ada:hunter2@127.0.0.1:1/db?sslmode=disable&connect_timeout=2",
		"postgres://ada:hunter2@127.0.0.1:notaport/db",
		"postgres://ada:hunter2@127.0.0.1/db?search_path=a;b",
	} {
		if _, err := taskindex.Open(ctx, u, t.TempDir()); err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("open %s: err = %v", u, err)
		}
		if _, _, err := taskindex.Version(ctx, u, t.TempDir()); err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("version err = %v", err)
		}
	}
}

func TestOpenRefusesANewerSchema(t *testing.T) {
	dir := t.TempDir()
	s, err := taskindex.Open(ctx, "", dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", sqlitePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tasks_schema_migrations (version, name, applied_at) VALUES (99, 'future.sql', 0)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := taskindex.Open(ctx, "", dir); err == nil || !strings.Contains(err.Error(), "99") || !strings.Contains(err.Error(), "upgrade") {
		t.Fatalf("err = %v", err)
	}
	if cur, _, err := taskindex.Version(ctx, "", dir); err != nil || cur != 99 {
		t.Fatalf("version = %d %v", cur, err)
	}
}

func TestVersionDoesNotMakeTheFile(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := taskindex.Version(ctx, "", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sqlitePath(dir)); err == nil {
		t.Fatal("Version made the database file")
	}
}

func TestPostgresSchemasAreSeparate(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	a, b := dbkittest.PostgresURL(t), dbkittest.PostgresURL(t)
	if cur, _, err := taskindex.Version(ctx, a, ""); err != nil || cur != 0 {
		t.Fatalf("version of a schema not made yet = %d %v", cur, err)
	}
	sa, err := taskindex.Open(ctx, a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sa.Close()
	sb, err := taskindex.Open(ctx, b, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close()
	put(t, sa, task("a", 1))
	inA, _ := sa.Count(ctx, taskindex.Filter{})
	inB, _ := sb.Count(ctx, taskindex.Filter{})
	if inA["queued"] != 1 || len(inB) != 0 {
		t.Fatal("two schemas share rows")
	}
	if cur, latest, err := taskindex.Version(ctx, a, ""); err != nil || cur != latest || cur < 1 {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestPostgresOpenedByTwoProcessesAtOnce(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	u := dbkittest.PostgresURL(t)
	var wg sync.WaitGroup
	stores := make([]taskindex.Store, 6)
	errs := make([]error, len(stores))
	for i := range stores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stores[i], errs[i] = taskindex.Open(ctx, u, "")
		}()
	}
	wg.Wait()
	for i, s := range stores {
		if errs[i] != nil {
			t.Fatalf("open %d: %v", i, errs[i])
		}
		defer s.Close()
	}
	if cur, latest, err := taskindex.Version(ctx, u, ""); err != nil || cur != latest {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestPostgresKeepsUsageAndTasksTogether(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	u := dbkittest.PostgresURL(t)
	first, err := taskindex.Open(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	db, err := sql.Open("pgx", u)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version BIGINT PRIMARY KEY)`); err != nil {
		t.Fatalf("a table of another component in the schema: %v", err)
	}
	s, err := taskindex.Open(ctx, u, "")
	if err != nil {
		t.Fatalf("reopen beside another component's tables: %v", err)
	}
	s.Close()
}
