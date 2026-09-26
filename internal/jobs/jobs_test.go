package jobs

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/evidence"

	"github.com/getlatentic/haas/internal/job"
)

func at(t *testing.T) *Index {
	t.Helper()
	index, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func queued(t *testing.T, index *Index, brief string) Record {
	t.Helper()
	record := Record{
		ID:      index.Free(brief, time.Now()),
		State:   Queued,
		Brief:   brief,
		Harness: "fixture",
		Asked:   "a-test",
		Queued:  Now(),
	}
	if err := os.MkdirAll(record.Dir(index.Root()).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestAJobsNameSaysWhenAndWhat(t *testing.T) {
	when := time.Date(2026, 9, 26, 14, 32, 0, 0, time.UTC)
	if got := Name("Fix the flaky login test, please", when); got != "20260926-1432-fix-the-flaky-login-test" {
		t.Fatalf("got %q", got)
	}
	if got := Name("", when); got != "20260926-1432" {
		t.Fatalf("a brief of nothing: %q", got)
	}
	if got := Name("  ///  ", when); got != "20260926-1432" {
		t.Fatalf("a brief of punctuation: %q", got)
	}
}

func TestTwoJobsAskedInTheSameMinuteGetDifferentNames(t *testing.T) {
	index := at(t)
	when := time.Now()
	first := queued(t, index, "fix the adder")
	second := index.Free("fix the adder", when)
	if first.ID == second {
		t.Fatalf("both are %q", first.ID)
	}
}

func TestOnlyOneWorkerTakesAJob(t *testing.T) {
	index := at(t)
	queued(t, index, "one")
	first, found, err := index.Claim()
	if err != nil || !found {
		t.Fatalf("the first claim should find it: %v %t", err, found)
	}
	if _, found, _ = index.Claim(); found {
		t.Fatal("a claimed job must not be handed out again")
	}
	if got, _ := index.Get(first.ID); got.State != Running {
		t.Fatalf("state is %q", got.State)
	}
}

func TestAJobRunningWhenTheServerStoppedIsInterrupted(t *testing.T) {
	index := at(t)
	record := queued(t, index, "left running")
	if _, _, err := index.Claim(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	got, found := reopened.Get(record.ID)
	if !found || got.State != Interrupted {
		t.Fatalf("want interrupted, got %q (found %t)", got.State, found)
	}
	if !got.State.Done() {
		t.Fatal("an interrupted job is not going to change on its own")
	}
}

func TestTheQueueRunsWhatIsQueuedAndRecordsHowItEnded(t *testing.T) {
	index := at(t)
	passing := queued(t, index, "passes")
	failing := queued(t, index, "fails")
	broken := queued(t, index, "cannot run")

	done := make(chan string, 3)
	queue := Start(index, func(dir job.Dir) (int, error) {
		defer func() { done <- dir.Path() }()
		switch {
		case dir.Path() == failing.Dir(index.Root()).Path():
			return 1, nil
		case dir.Path() == broken.Dir(index.Root()).Path():
			return 0, errors.New("no container runtime")
		}
		return 0, nil
	}, 2)
	defer queue.Stop()

	for taken := 0; taken < 3; taken++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the queue did not run everything that was queued")
		}
	}
	waitFor(t, index, passing.ID, Passed)
	waitFor(t, index, failing.ID, Failed)
	waitFor(t, index, broken.ID, Failed)
	if got, _ := index.Get(broken.ID); got.Exit != -1 {
		t.Fatalf("a job that could not run has no exit code of its own, got %d", got.Exit)
	}
}

func waitFor(t *testing.T, index *Index, id string, want State) {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		if got, _ := index.Get(id); got.State == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := index.Get(id)
	t.Fatalf("%s is %q, want %q", id, got.State, want)
}

// A container is a child of the container runtime, not of this server. A job
// whose server was killed often runs on, finishes, and writes its manifest — so
// the state file saying "running" is the stale thing, not the evidence.
func TestAJobThatFinishedWhileTheServerWasGoneKeepsItsVerdict(t *testing.T) {
	index := at(t)
	record := queued(t, index, "finished alone")
	if _, _, err := index.Claim(); err != nil {
		t.Fatal(err)
	}
	dir := record.Dir(index.Root())
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{
		Gate:   evidence.Verdict{Command: []string{"true"}, Code: 0, Passed: true},
		Signer: "a-test",
	}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reopened.Get(record.ID)
	if got.State != Passed {
		t.Fatalf("a job with a manifest saying it passed is %q", got.State)
	}
	if got.Exit != 0 {
		t.Fatalf("exit is %d, and the manifest says 0", got.Exit)
	}
}

func TestAJobThatLeftNoEvidenceIsInterruptedAndSaysNothingAboutItsExit(t *testing.T) {
	index := at(t)
	record := queued(t, index, "left nothing")
	if _, _, err := index.Claim(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reopened.Get(record.ID)
	if got.State != Interrupted {
		t.Fatalf("state is %q", got.State)
	}
	if got.Exit != -1 {
		t.Fatalf("exit is %d; zero would read as success", got.Exit)
	}
}

// Stopping must not start another job. systemd gives a stopping service time to
// finish what it is writing, not time to begin something new and have it killed.
func TestStoppingStartsNothingElse(t *testing.T) {
	index := at(t)
	for count := 0; count < 5; count++ {
		queued(t, index, "job")
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var taken atomic.Int32
	queue := Start(index, func(dir job.Dir) (int, error) {
		taken.Add(1)
		started <- struct{}{}
		<-release
		return 0, nil
	}, 1)

	<-started
	stopped := make(chan struct{})
	go func() { queue.Stop(); close(stopped) }()
	close(release)
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop never returned")
	}
	if ran := taken.Load(); ran != 1 {
		t.Fatalf("%d jobs ran; stopping should have started none after the first", ran)
	}
	queued := 0
	for _, record := range index.List() {
		if record.State == Queued {
			queued++
		}
	}
	if queued != 4 {
		t.Fatalf("%d jobs left queued, want 4", queued)
	}
}
