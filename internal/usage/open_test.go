package usage_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

func TestOpenSQLiteInTheDataDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "yet")
	if cur, latest, err := usage.Version(ctx, "", dir); err != nil || cur != 0 || latest < 1 {
		t.Fatalf("version before open = %d %d %v", cur, latest, err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("Version made the data directory")
	}
	s, err := usage.Open(ctx, "", dir)
	if err != nil {
		t.Fatal(err)
	}
	add(t, s, callRow("t", 0, "s", 1))
	s.Close()
	if _, err := os.Stat(sqlitePath(dir)); err != nil {
		t.Fatal(err)
	}
	s, err = usage.Open(ctx, "", dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := totals(t, s, usage.Filter{}).Calls; got != 1 {
		t.Fatalf("calls after reopen = %d", got)
	}
	if cur, latest, err := usage.Version(ctx, "", dir); err != nil || cur != latest || cur < 1 {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestOpenSQLiteURLNamesAnotherFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "elsewhere", "other.db")
	s, err := usage.Open(ctx, "sqlite://"+filepath.ToSlash(file), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRefusesUnknownSchemesAndListsTheForms(t *testing.T) {
	_, err := usage.Open(ctx, "mysql://root:hunter2@localhost/db", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "postgres://") || !strings.Contains(err.Error(), "sqlite://") {
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
		_, err := usage.Open(ctx, u, t.TempDir())
		if err == nil {
			t.Fatalf("%s opened", u)
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("the password is in %q", err)
		}
		if _, _, err := usage.Version(ctx, u, t.TempDir()); err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("version err = %v", err)
		}
	}
}

func TestOpenRefusesANewerSchema(t *testing.T) {
	dir := t.TempDir()
	s, err := usage.Open(ctx, "", dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", sqlitePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, 'future.sql', 0)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := usage.Open(ctx, "", dir); err == nil || !strings.Contains(err.Error(), "99") || !strings.Contains(err.Error(), "upgrade") {
		t.Fatalf("err = %v", err)
	}
	if cur, _, err := usage.Version(ctx, "", dir); err != nil || cur != 99 {
		t.Fatalf("version = %d %v", cur, err)
	}
}

func TestPostgresSchemasAreSeparate(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	a, b := freshSchemaURL(t), freshSchemaURL(t)
	if cur, _, err := usage.Version(ctx, a, ""); err != nil || cur != 0 {
		t.Fatalf("version of a schema not made yet = %d %v", cur, err)
	}
	sa, err := usage.Open(ctx, a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sa.Close()
	sb, err := usage.Open(ctx, b, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close()
	add(t, sa, callRow("t", 0, "s", 1))
	if totals(t, sa, usage.Filter{}).Calls != 1 || totals(t, sb, usage.Filter{}).Calls != 0 {
		t.Fatal("two schemas share rows")
	}
	if cur, latest, err := usage.Version(ctx, a, ""); err != nil || cur != latest || cur < 1 {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestPostgresOpenedByTwoProcessesAtOnce(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	u := freshSchemaURL(t)
	var wg sync.WaitGroup
	stores := make([]usage.Store, 6)
	errs := make([]error, len(stores))
	for i := range stores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stores[i], errs[i] = usage.Open(ctx, u, "")
		}()
	}
	wg.Wait()
	for i, s := range stores {
		if errs[i] != nil {
			t.Fatalf("open %d: %v", i, errs[i])
		}
		defer s.Close()
	}
	if cur, latest, err := usage.Version(ctx, u, ""); err != nil || cur != latest {
		t.Fatalf("version = %d %d %v", cur, latest, err)
	}
}

func TestPostgresRefusesANewerSchema(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	u := freshSchemaURL(t)
	s, err := usage.Open(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("pgx", u)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, 'future.sql', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := usage.Open(ctx, u, ""); err == nil || !strings.Contains(err.Error(), "upgrade") {
		t.Fatalf("err = %v", err)
	}
}
