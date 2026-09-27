package instance

import (
	"os"
	"path/filepath"
	"testing"
)

// leftByTheTwoStepStart is an instance as starting one used to leave it: a
// computer with a workspace and nothing said to it, and no conversation open.
func leftByTheTwoStepStart(t *testing.T, root, id string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	for _, each := range []string{"workspace", "state", "threads"} {
		if err := os.MkdirAll(filepath.Join(dir, each), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	held := Instance{ID: id, Harness: "opencode", Started: 1790000000}
	if err := writeJSON(filepath.Join(dir, "instance.json"), held); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Starting an instance is starting to talk to it, so there is never a moment
// when the store holds one with nowhere to talk.
func TestStartingAnInstanceOpensItsFirstConversation(t *testing.T) {
	store := opened(t)
	held, err := store.Start("20260926-1200-work", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	threads := store.Threads(held.ID)
	if len(threads) != 1 || store.Latest(held.ID).ID != threads[0].ID {
		t.Fatalf("a started instance holds %+v", threads)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(held.ID), "threads", "1.json")); err != nil {
		t.Errorf("its conversation was not written: %v", err)
	}
}

// An instance the two-step start left behind is given its conversation once,
// as the store loads, and the next load reads that same one from disk rather
// than opening another.
func TestAnInstanceWithNoConversationIsGivenOneOnceAsTheStoreLoads(t *testing.T) {
	root := t.TempDir()
	dir := leftByTheTwoStepStart(t, root, "20260926-1100-bare")

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	first := store.Latest("20260926-1100-bare")
	if first.ID == "" || first.Instance != "20260926-1100-bare" {
		t.Fatalf("the instance was given %+v", first)
	}
	if _, err := os.Stat(filepath.Join(dir, "threads", first.ID+".json")); err != nil {
		t.Fatalf("its conversation was not written: %v", err)
	}

	// Marked on disk, so a second load that opened a fresh one would show it.
	first.Title = "the one written on the first load"
	if err := writeJSON(filepath.Join(dir, "threads", first.ID+".json"), first); err != nil {
		t.Fatal(err)
	}
	again, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	threads := again.Threads("20260926-1100-bare")
	if len(threads) != 1 || threads[0].Title != first.Title {
		t.Errorf("a second load holds %+v, not the one the first load wrote", threads)
	}
}

// A directory that refuses the write still leaves an instance somewhere to
// talk. It is listed, and its conversation is held for as long as the process
// runs, rather than the instance disappearing from the list.
func TestAnInstanceWhoseDirectoryCannotBeWrittenStillHasAConversation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	root := t.TempDir()
	dir := leftByTheTwoStepStart(t, root, "20260926-1100-locked")
	threads := filepath.Join(dir, "threads")
	if err := os.Chmod(threads, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(threads, 0o755) })

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := store.Get("20260926-1100-locked"); !found {
		t.Fatal("an instance that cannot be written to was dropped from the store")
	}
	if store.Latest("20260926-1100-locked").ID == "" {
		t.Error("it has no conversation to go into")
	}
	if listing, _ := os.ReadDir(threads); len(listing) != 0 {
		t.Errorf("something was written through a read-only directory: %v", listing)
	}
}
