package datadir

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestADirectoryWithDataAndNoStampIsFormatOneToARollbackToo(t *testing.T) {
	var ran []string
	withFormats(t, Tasks, 2, noted(Tasks, 1, &ran, true))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20260101-job"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CanRollback(Tasks, dir, 2); err == nil {
		t.Error("a directory at format 1 cannot be rolled back to 2")
	}
	if err := Rollback(Tasks, dir, 1, "v0"); err != nil {
		t.Fatal(err)
	}
	if len(ran) > 0 {
		t.Errorf("a directory that was never upgraded had steps undone: %v", ran)
	}
	if stamp := stampOf(t, dir); stamp.Format != 1 {
		t.Errorf("stamp %+v", stamp)
	}
}

func TestTwoReleasesStampingADirectoryAtOnceBothStampIt(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 32*20)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				errs <- write(dir, 1, "v1")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if present(dir) {
		t.Error("stamping left something behind that reads as data")
	}
}
