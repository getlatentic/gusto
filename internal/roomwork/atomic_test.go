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
	actions, _ := ada.plan(ada.changes(now), now)
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
