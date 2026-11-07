package tasks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/pkg/task"
)

// queuedWithSpec is a queued task whose directory also holds the job.json a run would have been given.
func queuedWithSpec(t *testing.T, index *Index, brief string) Record {
	t.Helper()
	entry := queued(t, index, brief)
	spec := task.Spec{Brief: brief, Repo: "acme/app", Harness: "fixture", Model: "m-1", Session: "room-a", Thread: "2", Agent: "a1", Person: "ada@acme.example", Team: "core"}
	if err := spec.Write(entry.Dir(index.Root())); err != nil {
		t.Fatal(err)
	}
	return entry
}

// tear cuts a task's state.json in half, as a fault in the disk would.
func tear(t *testing.T, index *Index, entry Record) string {
	t.Helper()
	file := entry.Dir(index.Root()).Path() + "/state.json"
	whole, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, whole[:len(whole)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestATaskWhoseStateWasTornIsRebuiltAndTheTornFileIsKept(t *testing.T) {
	index := at(t)
	entry := queuedWithSpec(t, index, "was torn")
	file := tear(t, index, entry)
	torn, _ := os.ReadFile(file)

	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	got, found := reopened.Get(entry.ID)
	if !found {
		t.Fatal("a task whose state.json was torn is gone")
	}
	if got.State != Interrupted || got.Exit != -1 {
		t.Errorf("state %q exit %d: a torn task is not run again on a guess", got.State, got.Exit)
	}
	if got.Brief != "was torn" || got.Repo != "acme/app" || got.Instance != "room-a" || got.Thread != "2" || got.Agent != "a1" || got.Asked != "ada@acme.example" {
		t.Errorf("what job.json held was not carried over: %+v", got)
	}
	if got.Lane() != "room-a/a1" {
		t.Errorf("lane %q", got.Lane())
	}
	if _, err := read(entry.Dir(index.Root())); err != nil {
		t.Errorf("state.json is still unreadable: %v", err)
	}
	kept, _ := filepath.Glob(file + tornSuffix + "*")
	if len(kept) != 1 {
		t.Fatalf("the torn file was not kept: %v", kept)
	}
	if text, _ := os.ReadFile(kept[0]); string(text) != string(torn) {
		t.Error("the kept file is not the torn one")
	}
}

func TestATornTaskThatFinishedKeepsItsVerdict(t *testing.T) {
	index := at(t)
	entry := queuedWithSpec(t, index, "finished then torn")
	if _, _, err := index.Claim(); err != nil {
		t.Fatal(err)
	}
	dir := entry.Dir(index.Root())
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := record.Write(dir, record.Manifest{Check: &record.Verdict{Command: []string{"true"}, Passed: true}, Signer: "a-test", AtUnix: Now()}); err != nil {
		t.Fatal(err)
	}
	tear(t, index, entry)

	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Get(entry.ID); got.State != Passed || got.Exit != 0 {
		t.Fatalf("the verdict on disk was lost: %q, exit %d", got.State, got.Exit)
	}
}

func TestScanSeesATornTaskAsRebuiltAndLeavesItsFileAlone(t *testing.T) {
	index := at(t)
	entry := queuedWithSpec(t, index, "scanned while torn")
	file := tear(t, index, entry)
	torn, _ := os.ReadFile(file)

	all, err := Scan(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != entry.ID || all[0].Brief != "scanned while torn" {
		t.Fatalf("scan found %+v", all)
	}
	if now, _ := os.ReadFile(file); string(now) != string(torn) {
		t.Error("a read-only scan changed the file")
	}
}

// With no job.json to stand in for it a torn state.json cannot be rebuilt, and nothing is changed: the files stay for a person.
func TestATornTaskWithNothingToRebuildItFromIsLeftAsItIs(t *testing.T) {
	index := at(t)
	entry := queued(t, index, "no job.json")
	file := tear(t, index, entry)
	torn, _ := os.ReadFile(file)

	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	if _, found := reopened.Get(entry.ID); found {
		t.Error("a task that cannot be rebuilt was invented")
	}
	if now, _ := os.ReadFile(file); string(now) != string(torn) {
		t.Error("the torn file was changed")
	}
	if kept, _ := filepath.Glob(file + tornSuffix + "*"); len(kept) != 0 {
		t.Errorf("a file was set aside for a task that was not rebuilt: %v", kept)
	}
}

// A claim whose write fails must not say a task started.
func TestAClaimThatCouldNotBeWrittenIsNotCountedAsStarted(t *testing.T) {
	index := at(t)
	entry := queued(t, index, "cannot be written")
	if err := os.RemoveAll(entry.Dir(index.Root()).Path()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := index.Claim(); err == nil {
		t.Fatal("a claim whose state could not be written succeeded")
	}
	if started := index.Stats(time.Now()).LastStarted; started != 0 {
		t.Errorf("the server says a task started at %d", started)
	}
	if got, _ := index.Get(entry.ID); got.State != Queued {
		t.Errorf("the task is %q after a failed claim", got.State)
	}
}

func TestARemovedTaskIsGoneWhetherOrNotItsFilesCouldBeDeleted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root deletes what it is told not to")
	}
	index := at(t)
	entry := queued(t, index, "to remove")
	if _, _, err := index.Stop(entry.ID, "ada@acme.example"); err != nil {
		t.Fatal(err)
	}
	dir := entry.Dir(index.Root()).Path()
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "big"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(index.Root(), removing+entry.ID, "locked"), 0o755)

	if err := index.Remove(entry.ID); err != nil {
		t.Fatalf("a task whose files could not all be deleted was not removed: %v", err)
	}
	if _, found := index.Get(entry.ID); found {
		t.Error("the task is still in the index")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the task's directory is still in its place: %v", err)
	}
	if all, _ := Scan(index.Root()); len(all) != 0 {
		t.Errorf("a scan reads the half deleted task: %+v", all)
	}

	os.Chmod(filepath.Join(index.Root(), removing+entry.ID, "locked"), 0o755)
	reopened, err := Open(index.Root())
	if err != nil {
		t.Fatal(err)
	}
	if _, found := reopened.Get(entry.ID); found {
		t.Error("the removed task came back")
	}
	if _, err := os.Stat(filepath.Join(index.Root(), removing+entry.ID)); !os.IsNotExist(err) {
		t.Errorf("opening did not finish the deletion: %v", err)
	}
}
