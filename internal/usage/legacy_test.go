package usage_test

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

const legacyAppliedAt = 1700000000

var legacyMigrations = []struct {
	version int
	name    string
}{{1, "0001_usage.sql"}, {2, "0002_latest_index.sql"}}

// buildLegacy makes a database the way the migration runner before dbkit did: the version table, then each
// migration's SQL and its row, with the version table named schema_migrations.
func buildLegacy(t *testing.T, db *sql.DB, dialect string) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY, name TEXT NOT NULL, applied_at BIGINT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, m := range legacyMigrations {
		body, err := os.ReadFile(filepath.Join("migrations", dialect, m.name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		insert := `INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`
		if dialect == "postgres" {
			insert = `INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)`
		}
		if _, err := db.Exec(insert, m.version, m.name, legacyAppliedAt); err != nil {
			t.Fatal(err)
		}
	}
}

func checkLegacyOpens(t *testing.T, url, dir string, check *sql.DB) {
	t.Helper()
	if cur, latest, err := usage.Version(ctx, url, dir); err != nil || cur != 2 || latest != 2 {
		t.Fatalf("version of the old database = %d %d %v", cur, latest, err)
	}
	s, err := usage.Open(ctx, url, dir)
	if err != nil {
		t.Fatalf("an old database did not open: %v", err)
	}
	defer s.Close()
	add(t, s, callRow("t", 0, "s", 1))
	if got := totals(t, s, usage.Filter{}).Calls; got != 1 {
		t.Fatalf("calls = %d", got)
	}
	var n, minAt, maxAt int64
	if err := check.QueryRow(`SELECT count(*), MIN(applied_at), MAX(applied_at) FROM schema_migrations`).Scan(&n, &minAt, &maxAt); err != nil {
		t.Fatal(err)
	}
	if n != 2 || minAt != legacyAppliedAt || maxAt != legacyAppliedAt {
		t.Fatalf("the version table changed: %d rows, applied_at %d..%d", n, minAt, maxAt)
	}
}

func TestOpenTakesADatabaseTheOldRunnerMadeAtVersionTwo(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", sqlitePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	buildLegacy(t, db, "sqlite")
	checkLegacyOpens(t, "", dir, db)
}

func TestPostgresOpenTakesADatabaseTheOldRunnerMadeAtVersionTwo(t *testing.T) {
	if os.Getenv(postgresEnv) == "" {
		t.Skip(postgresEnv + " is not set")
	}
	u := freshSchemaURL(t)
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", os.Getenv(postgresEnv))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(`CREATE SCHEMA "` + parsed.Query().Get("search_path") + `"`); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", u)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	buildLegacy(t, db, "postgres")
	checkLegacyOpens(t, u, "", db)
}
