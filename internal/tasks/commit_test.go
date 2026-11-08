package tasks

import (
	"errors"
	"sync"
	"testing"
)

func claimedTask(t *testing.T, index *Index, brief string) Record {
	t.Helper()
	record := queued(t, index, brief)
	if _, found, err := index.Claim(); err != nil || !found {
		t.Fatal(found, err)
	}
	return record
}

func TestAStopAfterCommitIsRefusedAndTheTaskEndsAsItsWorkSays(t *testing.T) {
	index := at(t)
	record := claimedTask(t, index, "kept")
	if err := index.Commit(record.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := index.Stop(record.ID, "bob"); !errors.Is(err, ErrNotStoppable) {
		t.Fatalf("a stop after commit said %v", err)
	}
	if err := index.Finish(record.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := index.Get(record.ID); got.State != Passed || got.StoppedBy != "" || got.Exit != 0 {
		t.Fatalf("the task is %+v", got)
	}
}

func TestACommitAfterAStopIsRefusedAndTheTaskEndsInterrupted(t *testing.T) {
	index := at(t)
	record := claimedTask(t, index, "stopped")
	if _, _, err := index.Stop(record.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := index.Commit(record.ID); !errors.Is(err, ErrStoppedFirst) {
		t.Fatalf("a commit after a stop said %v", err)
	}
	if err := index.Finish(record.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := index.Get(record.ID); got.State != Interrupted || got.StoppedBy != "bob" {
		t.Fatalf("the task is %+v", got)
	}
}

func TestACommitOfATaskThatIsOverOrUnknownIsRefused(t *testing.T) {
	index := at(t)
	record := claimedTask(t, index, "over")
	if err := index.Finish(record.ID, Failed, 1); err != nil {
		t.Fatal(err)
	}
	if err := index.Commit(record.ID); !errors.Is(err, ErrEnded) {
		t.Errorf("commit of a task that is over said %v", err)
	}
	if err := index.Commit("nobody"); err == nil {
		t.Error("commit of an unknown task was accepted")
	}
}

// A stop and a commit made together are decided one way or the other, and the task ends as the winner says: a stop that won
// is interrupted and was not committed, and a commit that won was not stopped.
func TestAStopAndACommitMadeTogetherNeverBothSucceed(t *testing.T) {
	for round := 0; round < 200; round++ {
		index := at(t)
		record := claimedTask(t, index, "race")
		var stopErr, commitErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _, stopErr = index.Stop(record.ID, "bob") }()
		go func() { defer wg.Done(); commitErr = index.Commit(record.ID) }()
		wg.Wait()
		if (stopErr == nil) == (commitErr == nil) {
			t.Fatalf("round %d: stop %v, commit %v", round, stopErr, commitErr)
		}
	}
}
