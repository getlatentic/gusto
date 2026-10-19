package dbkit_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

var ctx = context.Background()

func TestOpenSQLiteInTheDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "yet")
	db, err := dbkit.Open(ctx, "", dir, "things.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := os.Stat(filepath.Join(dir, "things.db")); err != nil {
		t.Fatal(err)
	}
	if db.Dialect() != dbkit.SQLite {
		t.Fatalf("dialect = %s", db.Dialect())
	}
	if got := db.URLForLog(); !strings.HasPrefix(got, "sqlite://") || !strings.HasSuffix(got, "things.db") {
		t.Fatalf("URLForLog = %q", got)
	}
}

func TestOpenSQLiteURLNamesAnotherFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "elsewhere", "other.db")
	db, err := dbkit.Open(ctx, "sqlite://"+filepath.ToSlash(file), t.TempDir(), "x.db")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteReadersCannotWriteAndRunInWALMode(t *testing.T) {
	db := dbkittest.SQLite(t)
	if _, err := db.Reader().ExecContext(ctx, `CREATE TABLE x (a INTEGER)`); err == nil {
		t.Fatal("a reader wrote")
	}
	var mode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q %v", mode, err)
	}
	if got := db.Writer().Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("writer connections = %d", got)
	}
}

func TestOpenRefusesUnknownSchemesAndListsTheForms(t *testing.T) {
	_, err := dbkit.Open(ctx, "mysql://root:hunter2@localhost/db", t.TempDir(), "x.db")
	if err == nil || !strings.Contains(err.Error(), "postgres://") || !strings.Contains(err.Error(), "sqlite://") ||
		!strings.Contains(err.Error(), "x.db") {
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
		"postgres://ada@127.0.0.1:1/db?password=hunter2&sslmode=disable&connect_timeout=2",
	} {
		_, err := dbkit.Open(ctx, u, t.TempDir(), "x.db")
		if err == nil {
			t.Fatalf("%s opened", u)
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("the password is in %q", err)
		}
	}
}

func TestPostgresURLForLogHasNoPassword(t *testing.T) {
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		t.Skip(dbkittest.PostgresEnv + " is not set")
	}
	db := dbkittest.Postgres(t)
	got := db.URLForLog()
	if !strings.HasPrefix(got, "postgres://") || !strings.Contains(got, "search_path=dbkit_test_") {
		t.Fatalf("URLForLog = %q", got)
	}
}

func TestPostgresCreatesTheSchemaWhenTwoProcessesOpenTogether(t *testing.T) {
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		t.Skip(dbkittest.PostgresEnv + " is not set")
	}
	u := dbkittest.PostgresURL(t)
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := dbkit.Open(ctx, u, "", "")
			if err == nil {
				db.Close()
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
	}
}
