package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/task"
)

func TestAQueuedTaskThatIsStoppedNeverRunsAndEndsInterruptedByWhoStoppedIt(t *testing.T) {
	index := at(t)
	first, second := queued(t, index, "holds the only worker"), queued(t, index, "waits behind it")
	started, release := make(chan struct{}), make(chan struct{})
	ran := make(chan string, 4)
	queue := StartStoppable(index, func(ctx context.Context, dir task.Dir) (int, error) {
		ran <- dir.Path()
		if dir.Path() == first.Dir(index.Root()).Path() {
			close(started)
			<-release
		}
		return 0, nil
	}, 1)
	defer queue.Stop()
	<-started

	record, wasRunning, err := queue.Cancel(second.ID, "ada@acme.example")
	if err != nil || wasRunning || record.State != Interrupted || record.StoppedBy != "ada@acme.example" || record.Exit != -1 || record.Ended == 0 {
		t.Fatalf("%+v running=%v %v", record, wasRunning, err)
	}
	close(release)
	waitFor(t, index, first.ID, Passed)
	time.Sleep(150 * time.Millisecond)
	close(ran)
	var paths []string
	for path := range ran {
		paths = append(paths, path)
	}
	if len(paths) != 1 || paths[0] != first.Dir(index.Root()).Path() {
		t.Errorf("these ran, and only the first should have: %v", paths)
	}
	if got, _ := index.Get(second.ID); got.State != Interrupted {
		t.Errorf("the stopped job became %q", got.State)
	}
}

func TestARunningTaskThatIsStoppedIsToldToStopAndEndsInterruptedNotFailed(t *testing.T) {
	index := at(t)
	task1 := queued(t, index, "long")
	started := make(chan struct{})
	queue := StartStoppable(index, func(ctx context.Context, dir task.Dir) (int, error) {
		close(started)
		<-ctx.Done()
		return 1, errors.New("the harness was killed")
	}, 1)
	defer queue.Stop()
	<-started

	record, wasRunning, err := queue.Cancel(task1.ID, "ada@acme.example")
	if err != nil || !wasRunning || record.StoppedBy != "ada@acme.example" {
		t.Fatalf("%+v running=%v %v", record, wasRunning, err)
	}
	waitFor(t, index, task1.ID, Interrupted)
	got, _ := index.Get(task1.ID)
	if got.StoppedBy != "ada@acme.example" || got.Exit != -1 || got.Ended == 0 {
		t.Errorf("the stopped job is %+v", got)
	}
}

func TestATaskThatIsOverCannotBeStoppedAndAnUnknownOneIsNotFound(t *testing.T) {
	index := at(t)
	done := queued(t, index, "finishes")
	queue := Start(index, func(task.Dir) (int, error) { return 0, nil }, 1)
	defer queue.Stop()
	waitFor(t, index, done.ID, Passed)
	if _, _, err := queue.Cancel(done.ID, "ada"); !errors.Is(err, ErrNotStoppable) {
		t.Errorf("stopping a job that was over said %v", err)
	}
	if got, _ := index.Get(done.ID); got.State != Passed || got.StoppedBy != "" {
		t.Errorf("a job that was over was changed: %+v", got)
	}
	if _, _, err := queue.Cancel("no-such-job", "ada"); err == nil {
		t.Error("an unknown job was stopped")
	}
}

func TestAStopThatCameWhileTheTaskWasBeingClaimedStillStopsIt(t *testing.T) {
	index := at(t)
	task1 := queued(t, index, "raced")
	// The stop is recorded before any worker takes the task, and the task is taken as running by one that was already about to.
	if _, _, err := index.Stop(task1.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	if got, _ := index.Get(task1.ID); got.State != Interrupted {
		t.Fatalf("%q", got.State)
	}
}

// A readiness check can arrive the moment the queue starts, before its goroutines have been scheduled, and must find the
// workers it was started with.
func TestAQueueThatHasJustStartedAlreadyCountsItsWorkers(t *testing.T) {
	for range 200 {
		queue := Start(at(t), func(task.Dir) (int, error) { return 0, nil }, 3)
		if queue.Alive() != 3 || queue.Slots() != 3 {
			t.Fatalf("%d of %d workers", queue.Alive(), queue.Slots())
		}
		queue.Stop()
		if queue.Alive() != 0 {
			t.Fatalf("%d workers after stopping", queue.Alive())
		}
	}
}

// A stop that comes after the queue looked for one, and before the task's end is written, still ends it interrupted: a task
// somebody stopped is never recorded as having passed. A task that has ended is not ended again.
func TestAStopThatComesAsTheWorkReturnsStillEndsTheTaskInterrupted(t *testing.T) {
	index := at(t)
	record := queued(t, index, "stopped as it finishes")
	if _, _, err := index.Claim(); err != nil {
		t.Fatal(err)
	}
	if _, wasRunning, err := index.Stop(record.ID, "ada@acme.example"); err != nil || !wasRunning {
		t.Fatalf("running=%v %v", wasRunning, err)
	}
	if err := index.Finish(record.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	got, _ := index.Get(record.ID)
	if got.State != Interrupted || got.Exit != -1 || got.StoppedBy != "ada@acme.example" {
		t.Fatalf("a stopped task ended %+v", got)
	}
	if err := index.Finish(record.ID, Failed, 1); !errors.Is(err, ErrEnded) {
		t.Errorf("a task that had ended was ended again: %v", err)
	}
	if again, _ := index.Get(record.ID); again.State != Interrupted {
		t.Errorf("ending it again made it %q", again.State)
	}
}
