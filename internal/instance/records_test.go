package instance

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/getlatentic/gusto/internal/repoaccess"
)

// A thread file torn by a crash or a fault keeps its number, so the tasks that name it do not meet a new conversation.
func TestATornThreadFileKeepsItsNumberAndItsText(t *testing.T) {
	root := t.TempDir()
	store, _ := Open(root)
	held, err := store.Start("20261107-1000-work", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Said(held.ID, "1", "job-1", ""); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.threadsDir(held.ID), "1.json")
	whole, _ := os.ReadFile(file)
	if err := os.WriteFile(file, whole[:len(whole)/2], 0o644); err != nil {
		t.Fatal(err)
	}

	again, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	mended, found := again.Thread(held.ID, "1")
	if !found || mended.Title != damaged || len(mended.Turns) != 0 {
		t.Fatalf("thread 1 after the tear is %+v, found %v", mended, found)
	}
	next, err := again.NewThread(held.ID, "another")
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == "1" {
		t.Fatal("the torn thread's number was handed to a new conversation")
	}
	kept, _ := filepath.Glob(file + ".damaged-*")
	if len(kept) != 1 {
		t.Fatalf("the torn file was not kept: %v", kept)
	}
	if text, _ := os.ReadFile(kept[0]); string(text) != string(whole[:len(whole)/2]) {
		t.Error("the kept file is not the torn one")
	}
}

// A thread file nobody can read still has its number.
func TestAThreadFileThatCannotBeReadStillHoldsItsNumber(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads what it is told not to")
	}
	root := t.TempDir()
	store, _ := Open(root)
	held, _ := store.Start("20261107-1000-work", Instance{Harness: "opencode"})
	if _, err := store.NewThread(held.ID, "second"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.threadsDir(held.ID), "2.json")
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(file, 0o644)

	again, _ := Open(root)
	next, err := again.NewThread(held.ID, "third")
	if err != nil {
		t.Fatal(err)
	}
	if next.ID != "3" {
		t.Fatalf("the new conversation is %s", next.ID)
	}
}

// refuseWrites makes the threads folder of an instance unwritable, as a full or failing disk is, and puts it back afterwards.
func refuseWrites(t *testing.T, store *Store, id string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes where it is told not to")
	}
	dir := store.threadsDir(id)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
}

func TestATurnThatCouldNotBeWrittenIsNeitherKeptNorAnEvent(t *testing.T) {
	store := opened(t)
	room, _ := store.StartRoom("Launch plan", "ada@example.com", "")
	thread := store.Latest(room.ID)
	refuseWrites(t, store, room.ID)

	if err := store.Said(room.ID, thread.ID, "job-1", "named"); err == nil {
		t.Fatal("a write that failed was reported as done")
	}
	held, _ := store.Thread(room.ID, thread.ID)
	if len(held.Turns) != 0 || held.Remembered != "" {
		t.Errorf("the thread in memory is ahead of its file: %+v", held)
	}
	if got := eventsOf(t, store, room.ID); len(got) != 0 {
		t.Errorf("the log holds %v for a change that did not happen", got)
	}
}

func TestANoteThatCouldNotBeWrittenIsNeitherKeptNorAnEvent(t *testing.T) {
	store := opened(t)
	room, _ := store.StartRoom("Launch plan", "ada@example.com", "")
	thread := store.Latest(room.ID)
	refuseWrites(t, store, room.ID)

	if _, err := store.Note(room.ID, thread.ID, "ada@example.com", "hello"); err == nil {
		t.Fatal("a write that failed was reported as done")
	}
	held, _ := store.Thread(room.ID, thread.ID)
	if len(held.Notes) != 0 {
		t.Errorf("the thread in memory is ahead of its file: %+v", held)
	}
	if got := eventsOf(t, store, room.ID); len(got) != 0 {
		t.Errorf("the log holds %v for a change that did not happen", got)
	}
}

func TestWhatAThreadLearnedAndSawIsNotKeptWhenItCouldNotBeWritten(t *testing.T) {
	store := opened(t)
	room, _ := store.StartRoom("Launch plan", "ada@example.com", "")
	thread := store.Latest(room.ID)
	refuseWrites(t, store, room.ID)

	if err := store.Learned(room.ID, thread.ID, "a1", "mine"); err == nil {
		t.Fatal("a write that failed was reported as done")
	}
	if err := store.Saw(room.ID, thread.ID, "ada@example.com", 10); err == nil {
		t.Fatal("a write that failed was reported as done")
	}
	held, _ := store.Thread(room.ID, thread.ID)
	if len(held.Sessions) != 0 || len(held.Seen) != 0 {
		t.Errorf("the thread in memory is ahead of its file: %+v", held)
	}
}

// A start that fails leaves no instance in memory, and no directory that would hold its id for ever.
func TestAStartThatFailsLeavesNothingBehind(t *testing.T) {
	root := t.TempDir()
	store, _ := Open(root)
	t.Setenv(repoaccess.AppIDEnv, "an-id-without-a-key")

	if _, err := store.Start("20261107-1000-work", Instance{Harness: "opencode", Repo: "acme/app"}); err == nil {
		t.Fatal("a start whose clone could not be made succeeded")
	}
	if listed := store.List(); len(listed) != 0 {
		t.Errorf("the failed start is listed: %v", listed)
	}
	if _, err := os.Stat(filepath.Join(root, "20261107-1000-work")); !os.IsNotExist(err) {
		t.Errorf("the failed start left its directory: %v", err)
	}
	again, err := store.Start("20261107-1000-work", Instance{Harness: "opencode"})
	if err != nil || again.ID != "20261107-1000-work" {
		t.Fatalf("the id was not free again: %v, %v", again.ID, err)
	}
}

// A room made under the name of a removed one is a new room: its log does not carry the old room's window, and the removed
// room's id is not handed out.
func TestARemovedRoomsIdAndLogAreNotGivenToANewRoom(t *testing.T) {
	root := t.TempDir()
	store, _ := Open(root)
	old, _ := store.StartRoom("Launch plan", "ada@example.com", "")
	thread := store.Latest(old.ID)
	for i := range 3 {
		if err := store.Said(old.ID, thread.ID, "job-"+strconv.Itoa(i), ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Remove(old.ID); err != nil {
		t.Fatal(err)
	}

	fresh, err := store.StartRoom("Launch plan", "bob@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == old.ID {
		t.Fatal("the removed room's id was given to a new room")
	}
	if got := eventsOf(t, store, fresh.ID); len(got) != 0 {
		t.Errorf("the new room's log holds %v", got)
	}

	reopened, _ := Open(root)
	third, err := reopened.StartRoom("Launch plan", "cy@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == old.ID || third.ID == fresh.ID {
		t.Fatalf("after a restart the room is %s, which is taken", third.ID)
	}
}

// The log of a room that was removed and whose id is somehow made again starts empty in the same store: the cached log goes with
// the room.
func TestARemovedRoomsCachedLogIsDropped(t *testing.T) {
	store := opened(t)
	old, _ := store.StartRoom("Launch plan", "ada@example.com", "")
	_ = store.Said(old.ID, store.Latest(old.ID).ID, "job-1", "")
	before, err := store.RoomHead(old.ID)
	if err != nil || before.Seq == 0 {
		t.Fatalf("head %+v, %v", before, err)
	}
	if err := store.Remove(old.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir(old.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	head, err := store.events.Head(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	if head.Seq != 0 {
		t.Fatalf("the dropped log still holds %d events", head.Seq)
	}
}
