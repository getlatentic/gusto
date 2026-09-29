package jobs

import (
	"testing"
	"time"
)

// waiting queues a job that has waited for seconds.
func waiting(t *testing.T, index *Index, who, brief string, seconds int64) Record {
	t.Helper()
	record := by(t, index, who, "", brief)
	record.Queued = Now() - seconds
	index.guard.Lock()
	index.records[record.ID] = record
	index.guard.Unlock()
	return record
}

func TestTheQueueSaysWhatItHoldsAndHowLong(t *testing.T) {
	index := at(t)
	index.setSlots(3)
	waiting(t, index, "amy", "first", 30)
	waiting(t, index, "bo", "second", 90)
	claimed(t, index)
	stats := index.Stats(time.Now())
	if stats.ByState[Queued] != 1 || stats.ByState[Running] != 1 || stats.Slots != 3 {
		t.Fatalf("%+v", stats)
	}
	if stats.OldestWaitSeconds != 30 && stats.OldestWaitSeconds != 90 {
		t.Fatalf("oldest wait %d", stats.OldestWaitSeconds)
	}
	if stats.LastStarted == 0 {
		t.Error("a job started and the time was not kept")
	}
}

// A job a limit holds back is waiting as it should. One that nothing holds back,
// and has not started, is a queue that is not moving.
func TestAJobNothingHoldsBackIsStuckAndOneALimitHoldsIsNot(t *testing.T) {
	index := at(t)
	index.SetLimits(Limits{Person: 1})
	index.setSlots(3)
	waiting(t, index, "amy", "amy running", 10)
	claimed(t, index)
	waiting(t, index, "amy", "amy held by her limit", 500)
	if got := index.Stats(time.Now()).StuckSeconds; got != 0 {
		t.Fatalf("a job held by a limit was called stuck for %d s", got)
	}
	waiting(t, index, "bo", "bo could start", 200)
	if got := index.Stats(time.Now()).StuckSeconds; got < 199 || got > 202 {
		t.Fatalf("a job nothing held back, waiting 200 s, was stuck for %d s", got)
	}
}

func TestAFullPoolIsNotAStuckQueue(t *testing.T) {
	index := at(t)
	index.setSlots(1)
	waiting(t, index, "amy", "running", 10)
	claimed(t, index)
	waiting(t, index, "bo", "waiting for the slot", 900)
	if got := index.Stats(time.Now()).StuckSeconds; got != 0 {
		t.Fatalf("every runner busy was called stuck for %d s", got)
	}
}

func TestFinishedJobsAreCountedByHowTheyEnded(t *testing.T) {
	index := at(t)
	first := by(t, index, "amy", "a", "a")
	second := by(t, index, "amy", "b", "b")
	third := by(t, index, "amy", "c", "c")
	claimed(t, index)
	claimed(t, index)
	claimed(t, index)
	index.Finish(first.ID, Passed, 0)
	index.Finish(second.ID, Failed, 1)
	index.Finish(third.ID, Failed, 1)
	stats := index.Stats(time.Now())
	if stats.Finished[Passed] != 1 || stats.Finished[Failed] != 2 || stats.LastFinished == 0 {
		t.Fatalf("%+v", stats)
	}
}
