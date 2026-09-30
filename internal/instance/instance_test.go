package instance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func opened(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// Two threads on one instance are two conversations on one computer: the
// harness remembers each of them separately, and neither one's turns show up
// in the other.
func TestThreadsOnOneInstanceRememberSeparately(t *testing.T) {
	store := opened(t)
	held, err := store.Start("20260926-1200-work", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.NewThread(held.ID, "the bug")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.NewThread(held.ID, "the release")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("both threads are called %q", first.ID)
	}
	if err := store.Said(held.ID, first.ID, "job-a", "harness-session-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Said(held.ID, second.ID, "job-b", "harness-session-b"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ thread, turn, remembered string }{
		{first.ID, "job-a", "harness-session-a"},
		{second.ID, "job-b", "harness-session-b"},
	} {
		got, found := store.Thread(held.ID, want.thread)
		if !found {
			t.Fatalf("thread %s went missing", want.thread)
		}
		if len(got.Turns) != 1 || got.Turns[0] != want.turn {
			t.Errorf("thread %s holds %v", want.thread, got.Turns)
		}
		if got.Remembered != want.remembered {
			t.Errorf("thread %s is remembered as %q", want.thread, got.Remembered)
		}
	}
}

// An instance is a directory, so what it holds has to survive the process that
// made it.
func TestAnInstanceAndItsThreadsAreReadBackFromDisk(t *testing.T) {
	store := opened(t)
	held, err := store.Start("20260926-1200-work", Instance{Harness: "opencode", Model: "platform/qwen"})
	if err != nil {
		t.Fatal(err)
	}
	thread := store.Latest(held.ID)
	if err := store.Said(held.ID, thread.ID, "job-a", "harness-session-a"); err != nil {
		t.Fatal(err)
	}
	again, err := Open(store.Root())
	if err != nil {
		t.Fatal(err)
	}
	read, found := again.Get(held.ID)
	if !found || read.Harness != "opencode" || read.Model != "platform/qwen" {
		t.Fatalf("the instance came back as %+v", read)
	}
	threads := again.Threads(held.ID)
	if len(threads) != 1 || threads[0].Remembered != "harness-session-a" {
		t.Fatalf("its threads came back as %+v", threads)
	}
}

// A conversation written before threads existed is a computer with one
// conversation on it, and nothing it held is dropped on the way.
func TestAConversationWrittenWithoutThreadsBecomesAnInstanceWithOne(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "20260926-1100-old")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	was := map[string]any{
		"id": "20260926-1100-old", "harness": "opencode", "model": "platform/qwen",
		"repo": "git@github.com:you/repo.git", "remembered": "ses_123",
		"turns": []string{"job-a", "job-b"}, "startedAt": 1790000000,
	}
	text, err := json.MarshalIndent(was, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), text, 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	held, found := store.Get("20260926-1100-old")
	if !found {
		t.Fatal("the conversation did not come back as an instance")
	}
	if held.Harness != "opencode" || held.Repo != "git@github.com:you/repo.git" || held.Started != 1790000000 {
		t.Errorf("the instance came back as %+v", held)
	}
	threads := store.Threads(held.ID)
	if len(threads) != 1 {
		t.Fatalf("it came back with %d threads", len(threads))
	}
	if threads[0].Remembered != "ses_123" || len(threads[0].Turns) != 2 {
		t.Errorf("its thread came back as %+v", threads[0])
	}

	// Lifted once: the new shape is on disk, so the old file is never read again.
	if _, err := os.Stat(filepath.Join(dir, "instance.json")); err != nil {
		t.Errorf("the new shape was not written: %v", err)
	}
	// And a second open reads the new shape and agrees with the first.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if again := reopened.Threads(held.ID); len(again) != 1 || again[0].Remembered != "ses_123" {
		t.Errorf("a second open read %+v", again)
	}
}

// A number handed out twice would put two conversations in one file.
func TestAThreadNumberIsNotHandedOutTwice(t *testing.T) {
	store := opened(t)
	held, err := store.Start("20260926-1200-work", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{store.Latest(held.ID).ID: true}
	for range 5 {
		thread, err := store.NewThread(held.ID, "another")
		if err != nil {
			t.Fatal(err)
		}
		if seen[thread.ID] {
			t.Fatalf("thread %s was handed out twice", thread.ID)
		}
		seen[thread.ID] = true
	}
	if got := store.Threads(held.ID); len(got) != 6 {
		t.Errorf("the instance holds %d threads", len(got))
	}
}

func TestAThreadNeedsAnInstance(t *testing.T) {
	store := opened(t)
	if _, err := store.NewThread("nothing-here", "a thread"); err == nil {
		t.Fatal("a thread was opened on an instance that does not exist")
	}
}

// A turn says what it changed by diffing the workspace, so a workspace that is
// not a repository fails on the first thing anybody says to it. An instance
// started without a repository is the ordinary case, not an edge.
func TestAnInstanceStartedWithNoRepositoryStillHasSomethingToDiff(t *testing.T) {
	store := opened(t)
	held, err := store.Start("20260926-1200-empty", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	workspace := store.Workspace(held.ID)
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
		t.Fatalf("the workspace is not a repository: %v", err)
	}
	// And the diff a turn takes works there, which is the thing that failed.
	if err := os.WriteFile(filepath.Join(workspace, "new.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "add", "-N", ".")
	add.Dir = workspace
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add -N: %v: %s", err, output)
	}
	diff := exec.Command("git", "diff")
	diff.Dir = workspace
	patch, err := diff.Output()
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	if !strings.Contains(string(patch), "new.txt") {
		t.Errorf("the patch does not name what changed:\n%s", patch)
	}
}

// An instance whose workspace was made before a workspace was a repository is
// mended on the way in, so the next thing said to it is not the thing that
// discovers the problem.
func TestAWorkspaceThatIsNotARepositoryIsMendedOnTheWayIn(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	held, err := store.Start("20260926-1200-plain", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(store.Workspace(held.ID), ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.Workspace(held.ID), ".git")); err != nil {
		t.Errorf("the workspace was not mended: %v", err)
	}
}

func TestARemovedInstanceIsGoneButItsFilesAreKept(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start("one", Instance{Harness: "opencode"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Workspace("one"), "work.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove("one"); err != nil {
		t.Fatal(err)
	}
	if _, found := store.Get("one"); found {
		t.Error("a removed instance is still held")
	}
	again, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.List()) != 0 {
		t.Errorf("reopening the store found %v", again.List())
	}
	kept, _ := filepath.Glob(filepath.Join(root, removed, "one-*", "workspace", "work.txt"))
	if len(kept) != 1 {
		t.Error("the workspace should be moved aside, not deleted")
	}
}

func TestAThreadIsNamedByWhatWasFirstSaidInIt(t *testing.T) {
	if got := TitleOf("Draft a SOAP note from visit-notes.txt and keep it short, please, because it is long\nmore"); got != "Draft a SOAP note from visit-notes.txt and keep…" {
		t.Errorf("title %q", got)
	}
	store, _ := Open(t.TempDir())
	if _, err := store.Start("one", Instance{Harness: "dsh"}); err != nil {
		t.Fatal(err)
	}
	named, _ := store.NewThread("one", "chosen")
	store.Name("one", named.ID, "from the first message")
	store.Name("one", "1", "first")
	store.Name("one", "1", "second")
	if held, _ := store.Thread("one", named.ID); held.Title != "chosen" {
		t.Errorf("a chosen title was replaced with %q", held.Title)
	}
	if held, _ := store.Thread("one", "1"); held.Title != "first" {
		t.Errorf("the first message's title became %q", held.Title)
	}
}

// A template's files are where the harness starts from, committed, so the
// first turn's patch is what the harness changed and not the samples.
func TestSeededFilesAreCommittedAsTheStart(t *testing.T) {
	store, _ := Open(t.TempDir())
	if _, err := store.Start("one", Instance{Harness: "dsh"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Seed("one", map[string]string{"notes/visit.txt": "invented"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Seed("one", map[string]string{"../escape": "x"}); err == nil {
		t.Error("a file outside the folder was written")
	}
	status, _ := exec.Command("git", "-C", store.Workspace("one"), "status", "--porcelain").Output()
	if len(strings.TrimSpace(string(status))) != 0 {
		t.Errorf("the seeded files are not committed: %q", status)
	}
}

// Two instances proposed the same id, as two started in the same minute from
// one template are, become two instances, and the first keeps its harness.
func TestTwoInstancesProposedOneIdStayTwo(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Start("20260928-0950-engineering", Instance{Harness: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Start("20260928-0950-engineering", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("both instances are %s", first.ID)
	}
	if held, _ := store.Get(first.ID); held.Harness != "codex" {
		t.Errorf("the first instance's harness is now %q", held.Harness)
	}
}

func TestAnInstancesModelIsChangedAndKeptAcrossARestart(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	held, err := store.Start("20260930-1200-notes", Instance{Harness: "fixture", Model: "a/one"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetModel(held.ID, "a/two"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(held.ID); got.Model != "a/two" {
		t.Errorf("%+v", got)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Get(held.ID); got.Model != "a/two" {
		t.Errorf("the change was not kept: %+v", got)
	}
	if err := store.SetModel("no-such-instance", "a/two"); err == nil {
		t.Error("an instance that is not there has no model to change")
	}
}
