package roomwork

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurnsThatChangeDifferentFilesBothReachTheFolder(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"plan.md": "plan v1", "notes.md": "notes v1"})
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "plan.md", "plan v2 by Ada")
	write(t, ada.dest, "src/main.go", "package main")
	write(t, bea.dest, "notes.md", "notes v2 by Bea")
	os.Remove(filepath.Join(bea.dest, "plan.md"))
	write(t, bea.dest, "plan.md", "plan v1")

	if got := merge(t, ada); got.Changed != 2 || len(got.Conflicts) != 0 {
		t.Fatalf("Ada's merge gave %+v", got)
	}
	if got := merge(t, bea); got.Changed != 1 || len(got.Conflicts) != 0 {
		t.Fatalf("Bea's merge gave %+v", got)
	}
	want := "notes.md=notes v2 by Bea\nplan.md=plan v2 by Ada\nsrc/main.go=package main"
	if got := listing(t, in.Folder); got != want {
		t.Fatalf("the folder holds\n%s\nnot\n%s", got, want)
	}
	noStaging(t, in)
}

func TestAFileTwoTurnsChangedKeepsTheFirstAndSavesTheSecondBesideIt(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"report.md": "draft"})
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "report.md", "Ada's report")
	write(t, bea.dest, "report.md", "Bea's report")

	merge(t, ada)
	got := merge(t, bea)

	if len(got.Conflicts) != 1 {
		t.Fatalf("conflicts: %+v", got.Conflicts)
	}
	conflict := got.Conflicts[0]
	if conflict.Path != "report.md" || conflict.Side != "report.bea.md" || conflict.Agent != "Bea" || conflict.Other != "Ada" {
		t.Errorf("the conflict is %+v", conflict)
	}
	if read(t, in.Folder, "report.md") != "Ada's report" || read(t, in.Folder, "report.bea.md") != "Bea's report" {
		t.Errorf("the folder holds\n%s", listing(t, in.Folder))
	}
	words := got.Words()
	for _, part := range []string{"Ada and Bea both changed report.md", "report.md is Ada's version", "report.bea.md is Bea's version"} {
		if !strings.Contains(words, part) {
			t.Errorf("%q is not in %q", part, words)
		}
	}
	if got.Changed != 0 {
		t.Errorf("a saved side version counted as %d changes", got.Changed)
	}
}

func TestTwoTurnsThatWroteTheSameThingHaveNothingToSay(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"report.md": "draft"})
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "report.md", "final")
	write(t, bea.dest, "report.md", "final")
	merge(t, ada)
	if got := merge(t, bea); len(got.Conflicts) != 0 {
		t.Fatalf("identical files conflicted: %+v", got.Conflicts)
	}
	if !missing(in.Folder, "report.bea.md") {
		t.Error("a second copy of the same text was saved")
	}
}

func TestTwoTurnsThatCreatedTheSameNewFileConflict(t *testing.T) {
	hub, in := aRoom(t, nil)
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "todo.txt", "Ada's list")
	write(t, bea.dest, "todo.txt", "Bea's list")
	merge(t, ada)
	got := merge(t, bea)
	if len(got.Conflicts) != 1 || read(t, in.Folder, "todo.txt") != "Ada's list" || read(t, in.Folder, "todo.bea.txt") != "Bea's list" {
		t.Fatalf("%+v\n%s", got, listing(t, in.Folder))
	}
}

func TestADeletionDoesNotRemoveWhatAnotherTurnChanged(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"old.md": "v1", "gone.md": "bye"})
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "old.md", "v2 by Ada")
	os.Remove(filepath.Join(bea.dest, "old.md"))
	os.Remove(filepath.Join(bea.dest, "gone.md"))
	merge(t, ada)
	got := merge(t, bea)

	if read(t, in.Folder, "old.md") != "v2 by Ada" {
		t.Error("Bea's deletion removed Ada's change")
	}
	if !missing(in.Folder, "gone.md") {
		t.Error("a file only Bea deleted is still there")
	}
	if len(got.Conflicts) != 1 || !got.Conflicts[0].Deleted || got.Conflicts[0].Side != "" {
		t.Errorf("conflicts: %+v", got.Conflicts)
	}
	if words := got.Words(); !strings.Contains(words, "Bea deleted old.md while Ada changed it, so it stays.") {
		t.Errorf("the note reads %q", words)
	}
}

func TestAFileChangedAfterAnotherTurnDeletedItIsSavedBesideTheGap(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	os.Remove(filepath.Join(ada.dest, "a.md"))
	write(t, bea.dest, "a.md", "v2 by Bea")
	merge(t, ada)
	got := merge(t, bea)
	if !missing(in.Folder, "a.md") || read(t, in.Folder, "a.bea.md") != "v2 by Bea" || len(got.Conflicts) != 1 {
		t.Fatalf("%+v\n%s", got, listing(t, in.Folder))
	}
}

func TestAPersonsChangeToTheFolderIsKeptLikeAnAgents(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"brief.md": "v1"})
	bea := begin(t, hub, in, "Bea")
	write(t, in.Folder, "brief.md", "v2 by a person")
	write(t, bea.dest, "brief.md", "v2 by Bea")
	got := merge(t, bea)
	if read(t, in.Folder, "brief.md") != "v2 by a person" || read(t, in.Folder, "brief.bea.md") != "v2 by Bea" {
		t.Fatalf("%s", listing(t, in.Folder))
	}
	if got.Conflicts[0].Other != "" || !strings.Contains(got.Words(), "while somebody else did") {
		t.Errorf("%+v %q", got.Conflicts, got.Words())
	}
}

func TestSideVersionsOfTheSameFileGetNumbers(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada, bea, bea2 := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "a.md", "Ada")
	write(t, bea.dest, "a.md", "Bea one")
	write(t, bea2.dest, "a.md", "Bea two")
	merge(t, ada)
	merge(t, bea)
	merge(t, bea2)
	if read(t, in.Folder, "a.bea.md") != "Bea one" || read(t, in.Folder, "a.bea-2.md") != "Bea two" {
		t.Fatalf("%s", listing(t, in.Folder))
	}
}

func TestATurnThatChangedNothingMergesNothing(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada := begin(t, hub, in, "Ada")
	later := ada.copied["a.md"].mtime + 1
	_ = later
	if got := merge(t, ada); got.Changed != 0 || len(got.Conflicts) != 0 {
		t.Fatalf("%+v", got)
	}
	noStaging(t, in)
}

func TestARepositorysBookkeepingIsCopiedAndNotMerged(t *testing.T) {
	hub, in := aRoom(t, map[string]string{".git/HEAD": "ref: main", "a.md": "v1", "sub/.git/config": "x"})
	ada := begin(t, hub, in, "Ada")
	if read(t, ada.dest, ".git/HEAD") != "ref: main" {
		t.Fatal("the copy has no repository to work with")
	}
	write(t, ada.dest, ".git/HEAD", "ref: a-branch")
	write(t, ada.dest, ".git/objects/new", "commit")
	write(t, ada.dest, "sub/.git/config", "changed")
	write(t, ada.dest, "a.md", "v2")
	merge(t, ada)
	if read(t, in.Folder, ".git/HEAD") != "ref: main" || !missing(in.Folder, ".git/objects/new") || read(t, in.Folder, "sub/.git/config") != "x" {
		t.Error("a turn's repository bookkeeping reached the folder")
	}
	if read(t, in.Folder, "a.md") != "v2" {
		t.Error("the file change was not merged")
	}
}

func TestLinksAreMergedAsLinksAndNeverFollowed(t *testing.T) {
	hub, in := aRoom(t, nil)
	outside := t.TempDir()
	write(t, outside, "secret", "s")
	ada := begin(t, hub, in, "Ada")
	if err := os.Symlink(outside, filepath.Join(ada.dest, "away")); err != nil {
		t.Fatal(err)
	}
	merge(t, ada)
	if target, err := os.Readlink(filepath.Join(in.Folder, "away")); err != nil || target != outside {
		t.Fatalf("link: %q %v", target, err)
	}
	bea := begin(t, hub, in, "Bea")
	if seen, _ := scan(bea.dest); len(seen) != 1 || seen["away"].kind != link {
		t.Errorf("the copy followed the link: %v", seen)
	}
	os.Remove(filepath.Join(bea.dest, "away"))
	write(t, bea.dest, "away/inside.md", "x")
	write(t, in.Folder, "later.md", "z")
	got := merge(t, bea)
	if !missing(outside, "inside.md") {
		t.Fatal("a file was written through a link")
	}
	if len(got.Conflicts) != 1 || read(t, in.Folder, "away-inside.bea.md") != "x" || !missing(in.Folder, "away") {
		t.Errorf("%+v\n%v", got, listing(t, in.Folder))
	}
}

func TestAFileWhoseFolderIsAFileInTheRoomIsSavedAtTheTop(t *testing.T) {
	hub, in := aRoom(t, nil)
	ada, bea := begin(t, hub, in, "Ada"), begin(t, hub, in, "Bea")
	write(t, ada.dest, "docs", "a file")
	write(t, bea.dest, "docs/a.md", "text")
	merge(t, ada)
	got := merge(t, bea)
	if len(got.Conflicts) != 1 || read(t, in.Folder, "docs-a.bea.md") != "text" {
		t.Fatalf("%+v\n%s", got, listing(t, in.Folder))
	}
}

func TestAChangedModeAloneIsMerged(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"run.sh": "echo"})
	ada := begin(t, hub, in, "Ada")
	if err := os.Chmod(filepath.Join(ada.dest, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	merge(t, ada)
	info, err := os.Stat(filepath.Join(in.Folder, "run.sh"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("%v %v", info, err)
	}
}

func TestAFolderTheTurnEmptiedIsRemoved(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"old/one.md": "1", "old/deep/two.md": "2", "keep.md": "k"})
	ada := begin(t, hub, in, "Ada")
	os.RemoveAll(filepath.Join(ada.dest, "old"))
	merge(t, ada)
	if !missing(in.Folder, "old") || read(t, in.Folder, "keep.md") != "k" {
		t.Fatalf("%s", listing(t, in.Folder))
	}
}

func TestAFolderOverTheLimitsIsRefusedWithNothingLeft(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a": "12345", "b": "12345"})
	hub.limits = Limits{Bytes: 8, Files: 10}
	dest := filepath.Join(t.TempDir(), "workspace")
	_, err := hub.Begin(context.Background(), in, dest, "Ada")
	var big *TooBigError
	if !errors.As(err, &big) {
		t.Fatalf("Begin gave %v", err)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("the refused copy left %d entries", len(entries))
	}
	hub.limits = Limits{Bytes: 1 << 20, Files: 1}
	if _, err := hub.Begin(context.Background(), in, dest, "Ada"); !errors.As(err, &big) {
		t.Errorf("the file limit gave %v", err)
	}
	if len(hub.rooms) != 0 {
		t.Error("a refused turn is still counted in its room")
	}
}

func TestAMergeThatIsStoppedBeforeItStartsChangesNothing(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "v2")
	write(t, ada.dest, "b.md", "new")
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ada.Merge(stopped); err == nil {
		t.Fatal("a stopped merge went ahead")
	}
	if got := listing(t, in.Folder); got != "a.md=v1" {
		t.Fatalf("the folder holds %s", got)
	}
}

func TestATurnThatIsClosedWithoutMergingLeavesTheFolderAlone(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "half done")
	write(t, ada.dest, "junk.md", "x")
	ada.Close()
	ada.Close()
	if got := listing(t, in.Folder); got != "a.md=v1" {
		t.Fatalf("the folder holds %s", got)
	}
	if entries, _ := os.ReadDir(ada.dest); len(entries) != 0 {
		t.Error("the copy was not emptied")
	}
}
