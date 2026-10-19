// Package dbkittest runs a test body against every database dbkit supports.
package dbkittest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// PostgresEnv names the Postgres server the tests may use. Without it only SQLite is tested.
const PostgresEnv = "GUSTO_TEST_POSTGRES_URL"

var schemaCounter atomic.Int64

// Each runs body once against SQLite in a fresh directory and, when PostgresEnv is set, once against
// Postgres in a fresh schema that is dropped afterwards.
func Each(t *testing.T, body func(t *testing.T, db *dbkit.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { body(t, SQLite(t)) })
	if os.Getenv(PostgresEnv) == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) { body(t, Postgres(t)) })
}

// SQLite opens an empty SQLite database in a temporary directory.
func SQLite(t testing.TB) *dbkit.DB {
	t.Helper()
	db, err := dbkit.Open(context.Background(), "", t.TempDir(), "test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Postgres opens an empty schema on the server PostgresEnv names.
func Postgres(t testing.TB) *dbkit.DB {
	t.Helper()
	db, err := dbkit.Open(context.Background(), PostgresURL(t), "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// PostgresURL names a schema no other test run uses, on the server PostgresEnv names, and drops the schema
// when the test ends.
func PostgresURL(t testing.TB) string {
	t.Helper()
	base := os.Getenv(PostgresEnv)
	schema := fmt.Sprintf("dbkit_test_%d_%d_%d", os.Getpid(), time.Now().UnixNano()%1_000_000, schemaCounter.Add(1))
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal("bad " + PostgresEnv)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Cleanup(func() { dropSchema(t, base, schema) })
	return u.String()
}

func dropSchema(t testing.TB, base, schema string) {
	db, err := sql.Open("pgx", base)
	if err != nil {
		t.Error(err)
		return
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`); err != nil {
		t.Error(err)
	}
}
