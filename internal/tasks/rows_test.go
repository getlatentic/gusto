package tasks

import (
	"testing"

	"github.com/getlatentic/gusto/internal/taskindex"
)

func TestARowKeepsWhatLaneAndPayerNeed(t *testing.T) {
	turn := Record{ID: "t1", State: Queued, Brief: "b", Harness: "claude-code", Instance: "ins", Thread: "2", Agent: "scout",
		HandedBy: "writer", Answers: "t0", Hops: 2, Asked: "ada@acme.com", PaidBy: "owner@acme.com", Seq: 4, Queued: 9}
	back := RecordOf(RowOf(turn))
	if back != turn {
		t.Fatalf("got %+v, want %+v", back, turn)
	}
	if back.Lane() != "ins/scout" || back.Payer() != "owner@acme.com" || !back.InRoom() {
		t.Fatalf("lane %q, payer %q", back.Lane(), back.Payer())
	}
}

func TestARebuildDoesNotWriteAnOlderStateOverANewerOne(t *testing.T) {
	root := t.TempDir()
	index, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	task, _ := index.Get(queued(t, index, "ran while the rebuild read the files").ID)
	store, err := taskindex.Open(background, "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	finished := RowOf(task)
	finished.State, finished.Exit, finished.Started, finished.Ended = string(Passed), 0, task.Queued+1, task.Queued+5
	if err := store.Upsert(background, finished); err != nil {
		t.Fatal(err)
	}
	if _, err := Rebuild(background, store, root, false, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(background, task.ID); err != nil || got != finished {
		t.Fatalf("got %+v, %v; want %+v", got, err, finished)
	}
}
