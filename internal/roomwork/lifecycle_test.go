package roomwork

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// A merge whose undo failed leaves its backups as the only copy of the files it replaced. The next merge into the room must
// not clear them.
func TestAMergeAfterAFailedUndoDoesNotThrowTheBackupsAway(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes where it is told not to")
	}
	_, in := aRoom(t, map[string]string{"sub/b.md": "old b\n"})
	for _, dir := range []string{stageDir(in), backupDir(in)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := stage(in, []action{{dest: "sub/b.md", source: writeTemp(t, "new b\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(journalOf(in), journal{Steps: steps}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(journal{Steps: steps}); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(in.Folder, "sub")
	os.Chmod(sub, 0o500)
	defer os.Chmod(sub, 0o755)

	next := []action{{dest: "other.md", source: writeTemp(t, "other\n")}}
	if _, err := commit(in, next); err == nil {
		t.Fatal("a merge went ahead over files an earlier merge could not put back")
	}
	if !exists(journalOf(in)) || !exists(steps[0].Backup) {
		t.Fatal("the refused merge cleared the journal or the backups")
	}

	os.Chmod(sub, 0o755)
	if _, err := commit(in, next); err != nil {
		t.Fatal(err)
	}
	if read(t, in.Folder, "sub/b.md") != "old b\n" || read(t, in.Folder, "other.md") != "other\n" {
		t.Errorf("sub/b.md %q, other.md %q", read(t, in.Folder, "sub/b.md"), read(t, in.Folder, "other.md"))
	}
}

// Closing a turn that is waiting for the room's guard ends the wait, and the turn's emptied copy is never read as a merge.
func TestClosingATurnEndsItsMergeWaitingForTheGuard(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1", "b.md": "v1"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "v2")
	release, err := hub.hold(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() { _, err := ada.Merge(context.Background()); done <- err }()
	time.Sleep(20 * time.Millisecond)

	ada.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a merge of a closed turn went ahead")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a merge kept waiting for the guard after its turn was closed")
	}
	if got := listing(t, in.Folder); got != "a.md=v1\nb.md=v1" {
		t.Fatalf("the folder holds %s", got)
	}
}

func TestAMergedTurnIsNotMergedAgain(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "v2")
	if outcome := merge(t, ada); outcome.Changed != 1 {
		t.Fatalf("the first merge changed %d files", outcome.Changed)
	}
	outcome, err := ada.Merge(context.Background())
	if !errors.Is(err, ErrMerged) || len(outcome.Conflicts) != 0 {
		t.Fatalf("a second merge gave %+v, %v", outcome, err)
	}
	if got := listing(t, in.Folder); got != "a.md=v2" {
		t.Fatalf("the folder holds %s", got)
	}
}

func TestAMergeThatFailedMayBeTriedAgain(t *testing.T) {
	hub, in := aRoom(t, map[string]string{"a.md": "v1"})
	ada := begin(t, hub, in, "Ada")
	write(t, ada.dest, "a.md", "v2")
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ada.Merge(stopped); err == nil {
		t.Fatal("a stopped merge went ahead")
	}
	if outcome := merge(t, ada); outcome.Changed != 1 {
		t.Fatalf("the retry changed %d files", outcome.Changed)
	}
}

// A hub with no state for the room, as after a racing Close, answers with an error and not a nil dereference.
func TestAHubWithNoStateForARoomRefusesToHoldIt(t *testing.T) {
	hub, in := aRoom(t, nil)
	if _, err := hub.hold(context.Background(), in); !errors.Is(err, ErrClosed) {
		t.Fatalf("hold gave %v", err)
	}
}
