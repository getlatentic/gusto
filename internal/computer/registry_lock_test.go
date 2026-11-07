package computer

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/filelock"
)

// A person's computer is made once however many programs open the directory and ask at the same time, as the organization's
// server and gusto cloud do. getlatentic/planning#777.
func TestProgramsThatOpenTheSameDirectoryMakeOneComputerBetweenThem(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "computers")
	var wg sync.WaitGroup
	var mu sync.Mutex
	made := 0
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := Open(dir, "acme")
			if err != nil {
				t.Error(err)
				return
			}
			if _, new, err := r.Ensure("ada@acme.com", "", "cell-1"); err != nil {
				t.Error(err)
			} else if new {
				mu.Lock()
				made++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if made != 1 {
		t.Errorf("%d computers were made for one person", made)
	}
}

// A program that holds the directory's lock keeps another from changing a record, which says so and does not wait for ever.
func TestAChangeWaitsForAnotherProgramAndGivesUp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "computers")
	other, err := filelock.Try(filepath.Join(dir, lockFile))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir, "acme")
	if err != nil {
		t.Fatal(err)
	}
	r.lockWait = 100 * time.Millisecond
	if _, _, err := r.Ensure("ada@acme.com", "", "cell-1"); err == nil || !strings.Contains(err.Error(), "another program") {
		t.Errorf("a computer was made while another program held the record: %v", err)
	}
	if err := r.RetireAll(); err == nil {
		t.Error("a change was made while another program held the record")
	}
	other.Release()
	if _, made, err := r.Ensure("ada@acme.com", "", "cell-1"); err != nil || !made {
		t.Errorf("%v %v", made, err)
	}
}

// A computer left being made by a server that ended is missing once it has been so for too long, and not before.
func TestAComputerLeftBeingMadeIsMissingOnceItIsOverdue(t *testing.T) {
	r := open(t, "acme")
	now := time.Now()
	r.now = func() time.Time { return now }
	stuck, _, _ := r.Ensure("ada@acme.com", "", "cell-1")
	if drift, _ := r.Compare(nil); !drift.Clean() {
		t.Errorf("a computer being made is not missing: %+v", drift)
	}
	now = now.Add(MakingWithin + time.Minute)
	drift, _ := r.Compare(nil)
	if len(drift.Missing) != 1 || drift.Missing[0].ID != stuck.ID {
		t.Errorf("a computer that was never finished is missing: %+v", drift)
	}
}
