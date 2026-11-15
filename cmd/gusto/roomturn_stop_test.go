package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// stoppableTurn is a turn of the room's agent a1, queued in a real index and run by a real queue. It writes b.md in its copy of the
// room's folder.
type stoppableTurn struct {
	index *tasks.Index
	queue *tasks.Queue
	id    string
	room  string
}

// startStoppableTurn runs the turn. beforeMerge is called after the turn's work passed and before its changes are put in the
// room's folder, and after is called once they were, as the queue is between the work's return and the task's end.
func startStoppableTurn(t *testing.T, beforeMerge, after func(*stoppableTurn)) *stoppableTurn {
	t.Helper()
	rooms, held := aRoomWithTwoAgents(t, map[string]string{"a.md": "v1"})
	index, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	turn := &stoppableTurn{index: index, id: "turn-1", room: rooms.instances.AgentFolder(held.ID)}
	rooms.settle = index.Commit
	rooms.askedBy = func(task.Dir) string { beforeMerge(turn); return "" }
	dir := task.Dir(filepath.Join(index.Root(), turn.id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	_, spec := aRoomTurn(t, rooms, held, "a1")
	if err := index.Add(tasks.Record{ID: turn.id, State: tasks.Queued, Harness: "fixture", Asked: "ada@acme.example", Queued: tasks.Now()}); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	turn.queue = tasks.StartStoppable(index, func(ctx context.Context, d task.Dir) (int, error) {
		<-ready
		exit, err := rooms.around(ctx, spec, d, func() (int, error) {
			return 0, os.WriteFile(filepath.Join(d.WorkingFolder(), "b.md"), []byte("new"), 0o644)
		})
		after(turn)
		return exit, err
	}, 1)
	close(ready)
	t.Cleanup(turn.queue.Stop)
	return turn
}

func (s *stoppableTurn) waitFor(t *testing.T, want tasks.State) tasks.Record {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if got, _ := s.index.Get(s.id); got.State == want {
			return got
		}
	}
	got, _ := s.index.Get(s.id)
	t.Fatalf("the turn is %q, want %q", got.State, want)
	return got
}

func TestAStopAfterTheMergeLeavesTheTurnPassedAsItsDecisionSays(t *testing.T) {
	done, release := make(chan struct{}), make(chan struct{})
	turn := startStoppableTurn(t, func(*stoppableTurn) {}, func(*stoppableTurn) { close(done); <-release })
	var once sync.Once
	open := func() { once.Do(func() { close(release) }) }
	t.Cleanup(open)
	<-done
	if _, _, err := turn.queue.Cancel(turn.id, "bob@acme.example"); !errors.Is(err, tasks.ErrNotStoppable) {
		t.Fatalf("a stop after the merge said %v", err)
	}
	open()
	got := turn.waitFor(t, tasks.Passed)
	if got.StoppedBy != "" || got.Exit != 0 {
		t.Errorf("the turn is %+v", got)
	}
	if file(t, turn.room, "b.md") != "new" {
		t.Error("the changed file is not in the room's folder")
	}
}

func TestAStopBeforeTheMergeKeepsNothingAndEndsTheTurnInterrupted(t *testing.T) {
	turn := startStoppableTurn(t, func(turn *stoppableTurn) {
		if _, _, err := turn.queue.Cancel(turn.id, "bob@acme.example"); err != nil {
			t.Error(err)
		}
	}, func(*stoppableTurn) {})
	got := turn.waitFor(t, tasks.Interrupted)
	if got.StoppedBy != "bob@acme.example" {
		t.Errorf("the turn is %+v", got)
	}
	if _, err := os.Stat(filepath.Join(turn.room, "b.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a stopped turn was put in the room's folder: %v", err)
	}
}
