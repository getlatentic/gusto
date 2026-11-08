package roomwork

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// A merge that fails part way puts every file back as it was.
func TestAMergeThatFailsPartWayChangesNothing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes where it is told not to")
	}
	hub, in := aRoom(t, map[string]string{"a.md": "a1", "locked/b.md": "b1", "z.md": "z1", "gone.md": "g"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "a2")
	write(t, ada.dest, "locked/b.md", "b2")
	write(t, ada.dest, "locked/new/c.md", "c")
	write(t, ada.dest, "z.md", "z2")
	write(t, ada.dest, "fresh/d.md", "d")
	os.Remove(filepath.Join(ada.dest, "gone.md"))
	before := listing(t, in.Folder)
	locked := filepath.Join(in.Folder, "locked")
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)

	if _, err := ada.Merge(t.Context()); err == nil {
		t.Fatal("the merge went ahead into a folder it cannot write")
	}
	os.Chmod(locked, 0o755)
	if got := listing(t, in.Folder); got != before {
		t.Fatalf("the folder was left half merged:\n%s\nwas\n%s", got, before)
	}
	if !missing(in.Folder, "fresh") || !missing(in.Folder, "locked/new") {
		t.Error("folders the merge made were left behind")
	}
	noStaging(t, in)
}

// A server that died in the middle of a merge leaves a journal; the next turn in the room puts the folder back first.
func TestAMergeThatWasCutOffIsUndoneBeforeTheNextTurnBegins(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "a1", "b.md": "b1", "gone.md": "g"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "a2")
	write(t, ada.dest, "b.md", "b2")
	write(t, ada.dest, "new/c.md", "c")
	os.Remove(filepath.Join(ada.dest, "gone.md"))
	before := listing(t, in.Folder)

	now, _ := scan(ada.dest)
	actions := ada.plan(ada.changes(now), now).actions
	for _, dir := range []string{stageDir(in), backupDir(in)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := stage(in, actions)
	if err != nil {
		t.Fatal(err)
	}
	made, err := makeParents(in.Folder, steps)
	if err != nil {
		t.Fatal(err)
	}
	cut := journal{Steps: steps, Made: made}
	if err := atomicfile.Write(journalOf(in), cut, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, each := range cut.Steps[:len(cut.Steps)-1] {
		if err := each.do(); err != nil {
			t.Fatal(err)
		}
	}
	if listing(t, in.Folder) == before {
		t.Fatal("the cut-off merge changed nothing, so this test proves nothing")
	}

	fresh := NewHub(roomy)
	next, err := fresh.Begin(t.Context(), in, filepath.Join(t.TempDir(), "w"), "Bea")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if got := listing(t, in.Folder); got != before {
		t.Fatalf("the cut-off merge was not undone:\n%s\nwas\n%s", got, before)
	}
	if read(t, next.dest, "a.md") != "a1" {
		t.Error("the new turn began from a half merged folder")
	}
	noStaging(t, in)
}

// A recovery that cannot put every file back keeps the journal and the backups, and a later one finishes without removing a
// file the first put back. spec/MergeJournal.tla is the model.
func TestARecoveryThatFailsPartWayIsFinishedLater(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes where it is told not to")
	}
	_, in := aRoom(t, map[string]string{"a.md": "old a\n", "sub/b.md": "old b\n"})
	for _, dir := range []string{stageDir(in), backupDir(in)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := stage(in, []action{{dest: "a.md", source: writeTemp(t, "new a\n")}, {dest: "sub/b.md", source: writeTemp(t, "new b\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(journalOf(in), journal{Steps: steps}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(journal{Steps: steps}); err != nil {
		t.Fatal(err)
	}
	// The merge is cut off before its journal goes, and sub/ cannot be written to when the room is next used.
	sub := filepath.Join(in.Folder, "sub")
	os.Chmod(sub, 0o500)
	if err := recoverMerge(in); err == nil {
		t.Fatal("a recovery that could not put sub/b.md back said it had")
	}
	os.Chmod(sub, 0o755)
	if read(t, in.Folder, "a.md") != "old a\n" || !exists(journalOf(in)) {
		t.Fatalf("after the failed recovery a.md holds %q, journal kept %v", read(t, in.Folder, "a.md"), exists(journalOf(in)))
	}
	if err := recoverMerge(in); err != nil {
		t.Fatal(err)
	}
	if read(t, in.Folder, "a.md") != "old a\n" || read(t, in.Folder, "sub/b.md") != "old b\n" {
		t.Errorf("a.md %q, sub/b.md %q", read(t, in.Folder, "a.md"), read(t, in.Folder, "sub/b.md"))
	}
	noStaging(t, in)
}

func writeTemp(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
