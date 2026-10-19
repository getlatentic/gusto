package usage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/usage/usagetest"
)

const postgresEnv = dbkittest.PostgresEnv

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

func freshSchemaURL(t testing.TB) string { return dbkittest.PostgresURL(t) }

func sqlitePath(dir string) string { return filepath.Join(dir, "usage.db") }
