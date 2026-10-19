package usage_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
)

const postgresEnv = "GUSTO_TEST_POSTGRES_URL"

var schemaCounter atomic.Int64

type backend struct {
	name string
	open func(t testing.TB) usage.Store
}

// backends are the stores the contract holds for. Postgres is there only when a server is named.
func backends(t testing.TB) []backend {
	list := []backend{
		{"fake", func(testing.TB) usage.Store { return usagetest.New() }},
		{"sqlite", openSQLite},
	}
	if os.Getenv(postgresEnv) != "" {
		list = append(list, backend{"postgres", openPostgres})
	}
	return list
}

func openSQLite(t testing.TB) usage.Store {
	t.Helper()
	s, err := usage.Open(context.Background(), "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func openPostgres(t testing.TB) usage.Store {
	t.Helper()
	s, err := usage.Open(context.Background(), freshSchemaURL(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// freshSchemaURL names a schema no other test run uses and drops it when the test ends.
func freshSchemaURL(t testing.TB) string {
	t.Helper()
	base := os.Getenv(postgresEnv)
	schema := fmt.Sprintf("usage_test_%d_%d_%d", os.Getpid(), time.Now().UnixNano()%1_000_000, schemaCounter.Add(1))
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal("bad " + postgresEnv)
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

func sqlitePath(dir string) string { return filepath.Join(dir, "usage.db") }
