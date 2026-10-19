package taskindex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
	"github.com/getlatentic/gusto/internal/taskindex"
	"github.com/getlatentic/gusto/internal/taskindex/taskindextest"
)

const postgresEnv = dbkittest.PostgresEnv

var ctx = context.Background()

type backend struct {
	name string
	open func(t testing.TB) taskindex.Store
}

// backends are the stores the contract holds for. Postgres is there only when a server is named.
func backends() []backend {
	list := []backend{
		{"fake", func(testing.TB) taskindex.Store { return taskindextest.New() }},
		{"sqlite", openSQLite},
	}
	if os.Getenv(postgresEnv) != "" {
		list = append(list, backend{"postgres", openPostgres})
	}
	return list
}

func openSQLite(t testing.TB) taskindex.Store {
	t.Helper()
	s, err := taskindex.Open(ctx, "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func openPostgres(t testing.TB) taskindex.Store {
	t.Helper()
	s, err := taskindex.Open(ctx, dbkittest.PostgresURL(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sqlitePath(dir string) string { return filepath.Join(dir, "tasks.db") }
