package roomwork

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const plan = "# Launch plan\n\n## Goals\nlaunch in May\n\n## Risks\nnone yet\n\n## Owners\nAda\n"

func TestTwoTurnsThatChangedDifferentLinesOfOneFileAreJoined(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"plan.md": plan})
	scout, draft := begin(t, hub, in, "Scout"), begin(t, hub, in, "Draft")
	write(t, scout.dest, "plan.md", strings.Replace(plan, "launch in May", "launch in June", 1))
	write(t, draft.dest, "plan.md", strings.Replace(plan, "none yet", "the venue may cancel", 1))

	merge(t, scout)
	got := merge(t, draft)

	want := strings.Replace(strings.Replace(plan, "launch in May", "launch in June", 1), "none yet", "the venue may cancel", 1)
	if read(t, in.Folder, "plan.md") != want {
		t.Fatalf("plan.md holds\n%s", read(t, in.Folder, "plan.md"))
	}
	if len(got.Conflicts) != 0 || len(got.Joined) != 1 || got.Changed != 1 {
		t.Fatalf("the merge gave %+v", got)
	}
	if words := got.Words(); words != "Scout and Draft both changed plan.md, in different places, and it holds both changes." {
		t.Errorf("the room is told %q", words)
	}
	if !missing(in.Folder, "plan.draft.md") {
		t.Error("a joined file was also saved beside")
	}
	noStaging(t, in)
}

func TestTwoTurnsThatChangedTheSameLineStillConflict(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"plan.md": plan})
	scout, draft := begin(t, hub, in, "Scout"), begin(t, hub, in, "Draft")
	write(t, scout.dest, "plan.md", strings.Replace(plan, "launch in May", "launch in June", 1))
	write(t, draft.dest, "plan.md", strings.Replace(plan, "launch in May", "launch in July", 1))

	merge(t, scout)
	got := merge(t, draft)

	if len(got.Conflicts) != 1 || len(got.Joined) != 0 {
		t.Fatalf("the merge gave %+v", got)
	}
	if !strings.Contains(read(t, in.Folder, "plan.md"), "June") || !strings.Contains(read(t, in.Folder, "plan.draft.md"), "July") {
		t.Errorf("the folder holds\n%s", listing(t, in.Folder))
	}
}

func TestAFileThatIsNotTextConflictsWhole(t *testing.T) {
	binary := "\x00\x01one\n\x00two\n"
	hub, in := aRoom(t, map[string]string{"data.bin": binary})
	scout, draft := begin(t, hub, in, "Scout"), begin(t, hub, in, "Draft")
	write(t, scout.dest, "data.bin", strings.Replace(binary, "one", "ONE", 1))
	write(t, draft.dest, "data.bin", strings.Replace(binary, "two", "TWO", 1))

	merge(t, scout)
	if got := merge(t, draft); len(got.Conflicts) != 1 || len(got.Joined) != 0 {
		t.Fatalf("the merge gave %+v", got)
	}
}

func TestAPersonsEditIsJoinedWithATurnThatChangedOtherLines(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"plan.md": plan})
	scout := begin(t, hub, in, "Scout")
	write(t, scout.dest, "plan.md", strings.Replace(plan, "launch in May", "launch in June", 1))

	done, err := hub.Hold(context.Background(), in.Folder, []string{"plan.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(in.Folder, "plan.md", plan+"Bea\n"); err != nil {
		t.Fatal(err)
	}
	done(true)
	got := merge(t, scout)

	want := strings.Replace(plan, "launch in May", "launch in June", 1) + "Bea\n"
	if read(t, in.Folder, "plan.md") != want || len(got.Joined) != 1 {
		t.Fatalf("plan.md holds\n%s\nand the merge gave %+v", read(t, in.Folder, "plan.md"), got)
	}
	if words := got.Words(); !strings.Contains(words, "Scout changed plan.md while somebody else did, in different places") {
		t.Errorf("the room is told %q", words)
	}
}

func TestAChangeThatWasNotMadeLeavesNothingKept(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"plan.md": plan})
	begin(t, hub, in, "Scout")
	done, err := hub.Hold(context.Background(), in.Folder, []string{"plan.md"})
	if err != nil {
		t.Fatal(err)
	}
	done(false)
	if kept, _ := filepath.Glob(filepath.Join(priorDir(in), "*")); len(kept) != 0 {
		t.Errorf("a failed change kept %v", kept)
	}
}

// A person's change waits for a merge that is changing the folder, so the merge cannot put its file over one the person just
// wrote, and the person's file is never lost.
func TestAChangeWaitsForAMergeAndAMergeForAChange(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"plan.md": plan})
	scout := begin(t, hub, in, "Scout")
	write(t, scout.dest, "notes.md", "Scout's notes\n")

	release, err := hub.hold(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	wrote := make(chan error, 1)
	go func() {
		done, err := hub.Hold(context.Background(), in.Folder, nil)
		if err == nil {
			err = os.WriteFile(filepath.Join(in.Folder, "notes.md"), []byte("Ada's upload\n"), 0o644)
			done(err == nil)
		}
		wrote <- err
	}()
	select {
	case <-wrote:
		t.Fatal("the change did not wait for the merge")
	default:
	}
	release()
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
	got := merge(t, scout)
	if read(t, in.Folder, "notes.md") != "Ada's upload\n" || len(got.Conflicts) != 1 || read(t, in.Folder, got.Conflicts[0].Side) != "Scout's notes\n" {
		t.Fatalf("the folder holds\n%s\nand the merge gave %+v", listing(t, in.Folder), got)
	}
}

func TestAChangeWithNoTurnWorkingIsHeldAtOnce(t *testing.T) {
	hub, in := aRoom(t, nil)
	done, err := hub.Hold(context.Background(), in.Folder, []string{"a.md"})
	if err != nil {
		t.Fatal(err)
	}
	done(true)
}

func replaceFile(folder, name, text string) error {
	next := filepath.Join(folder, name+".next")
	if err := os.WriteFile(next, []byte(text), 0o644); err != nil {
		return err
	}
	return os.Rename(next, filepath.Join(folder, name))
}
