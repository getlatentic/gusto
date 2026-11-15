package dbkit

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestBusyIsTheRefusalOfAFileAnotherConnectionHolds(t *testing.T) {
	file := filepath.Join(t.TempDir(), "held.db")
	holder, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	other, err := sql.Open("sqlite", file+"?_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := holder.Exec(`CREATE TABLE t (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	held, err := holder.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer held.Rollback()
	if _, err := held.Exec(`INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	_, refused := other.Exec(`INSERT INTO t VALUES (2)`)
	if refused == nil {
		t.Fatal("a second writer was let in while the first held the file")
	}
	if !Busy(fmt.Errorf("dbkit: open: %w", refused)) {
		t.Fatalf("Busy did not know the refusal %v", refused)
	}
	if Busy(errors.New("SQLITE_BUSY")) {
		t.Fatal("Busy took text for SQLite's answer")
	}
}
