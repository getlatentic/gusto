package dbkit

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openRaw(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.Context(), "", t.TempDir(), "t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
