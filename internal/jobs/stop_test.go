package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/job"
)

func TestAQueuedJobThatIsStoppedNeverRunsAndEndsInterruptedByWhoStoppedIt(t *testing.T) {
	index := at(t)
	first, second := queued(t, index, "holds the only worker"), queued(t, index, "waits behind it")
	started, release := make(chan struct{}), make(chan struct{})
	ran := make(chan string, 4)
	queue := StartStoppable(index, func(ctx context.Context, dir job.Dir) (int, error) {
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

func TestARunningJobThatIsStoppedIsToldToStopAndEndsInterruptedNotFailed(t *testing.T) {
	index := at(t)
	job1 := queued(t, index, "long")
	started := make(chan struct{})
	queue := StartStoppable(index, func(ctx context.Context, dir job.Dir) (int, error) {
		close(started)
		<-ctx.Done()
		return 1, errors.New("the harness was killed")
	}, 1)
	defer queue.Stop()
	<-started

	record, wasRunning, err := queue.Cancel(job1.ID, "ada@acme.example")
	if err != nil || !wasRunning || record.StoppedBy != "ada@acme.example" {
		t.Fatalf("%+v running=%v %v", record, wasRunning, err)
	}
	waitFor(t, index, job1.ID, Interrupted)
	got, _ := index.Get(job1.ID)
	if got.StoppedBy != "ada@acme.example" || got.Exit != -1 || got.Ended == 0 {
		t.Errorf("the stopped job is %+v", got)
	}
}

func TestAJobThatIsOverCannotBeStoppedAndAnUnknownOneIsNotFound(t *testing.T) {
	index := at(t)
	done := queued(t, index, "finishes")
	queue := Start(index, func(job.Dir) (int, error) { return 0, nil }, 1)
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

func TestAStopThatCameWhileTheJobWasBeingClaimedStillStopsIt(t *testing.T) {
	index := at(t)
	job1 := queued(t, index, "raced")
	// The stop is recorded before any worker takes the job, and the job is taken as running by one that was already about to.
	if _, _, err := index.Stop(job1.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	if got, _ := index.Get(job1.ID); got.State != Interrupted {
		t.Fatalf("%q", got.State)
	}
}
