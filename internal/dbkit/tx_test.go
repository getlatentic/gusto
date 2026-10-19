package dbkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRetryableRecognisesRaceLosses(t *testing.T) {
	for code, want := range map[string]bool{"40001": true, "40P01": true, "23505": false} {
		err := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code})
		if retryable(err) != want {
			t.Errorf("postgres %s retryable = %v", code, !want)
		}
	}
	if retryable(context.Canceled) || retryable(errors.New("plain")) {
		t.Error("an error that a rerun cannot fix is retryable")
	}
}

func TestRetryableRecognisesABusySQLiteDatabase(t *testing.T) {
	path := t.TempDir() + "/busy.db"
	hold, err := openRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	holder, err := hold.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if _, err := holder.Exec(`CREATE TABLE a (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	other, err := openRaw(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	_, err = other.Exec(`CREATE TABLE b (x INTEGER)`)
	if err == nil || !retryable(err) {
		t.Fatalf("err = %v, retryable = %v", err, err != nil && retryable(err))
	}
}

func TestInTxRunsAgainWhenItLosesARace(t *testing.T) {
	db := openTest(t)
	calls := 0
	err := db.InTx(context.Background(), func(Tx) error {
		calls++
		if calls < 3 {
			return &pgconn.PgError{Code: "40001"}
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("calls = %d err = %v", calls, err)
	}
}

func TestInTxGivesUpAfterTheAttemptsAndOnOtherErrors(t *testing.T) {
	db := openTest(t)
	calls := 0
	err := db.InTx(context.Background(), func(Tx) error { calls++; return &pgconn.PgError{Code: "40001"} })
	if err == nil || calls != maxAttempts {
		t.Fatalf("calls = %d err = %v", calls, err)
	}
	calls = 0
	boom := errors.New("boom")
	if err := db.InTx(context.Background(), func(Tx) error { calls++; return boom }); !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("calls = %d err = %v", calls, err)
	}
}

func TestInTxWritersDoNotLoseUpdates(t *testing.T) {
	db := openTest(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE counter (n INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO counter VALUES (0)`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.InTx(ctx, func(tx Tx) error {
				var n int
				if err := tx.QueryRowContext(ctx, `SELECT n FROM counter`).Scan(&n); err != nil {
					return err
				}
				_, err := tx.ExecContext(ctx, `UPDATE counter SET n = ?`, n+1)
				return err
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var n int
	if err := db.QueryRowContext(ctx, `SELECT n FROM counter`).Scan(&n); err != nil || n != 20 {
		t.Fatalf("n = %d %v", n, err)
	}
}
