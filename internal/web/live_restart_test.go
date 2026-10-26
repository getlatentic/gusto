package web

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/roomlive"
)

func TestATurnThatWasRunningWhenTheServerStoppedIsAnnouncedToItsRoomOnTheNextStart(t *testing.T) {
	root, sessions := t.TempDir(), t.TempDir()
	instances, err := instance.Open(sessions)
	if err != nil {
		t.Fatal(err)
	}
	room, _ := instances.StartRoom("Launch plan", alice)
	thread := instances.Latest(room.ID)

	first, err := jobs.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := first.Reserve("long job", time.Now())
	record := jobs.Record{ID: id, State: jobs.Queued, Brief: "long job", Harness: "fixture", Instance: room.ID, Thread: thread.ID, Agent: "a1", Queued: jobs.Now()}
	if err := os.MkdirAll(record.Dir(root).Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := first.Add(record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.Claim(); err != nil {
		t.Fatal(err)
	}

	reopened, err := jobs.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(reopened, instances, nil, password, &Identity{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UseLive(roomlive.Options{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.CloseLive(ctx); err != nil {
		t.Fatal(err)
	}
	slice, _ := instances.RoomEvents(room.ID, 0, 0)
	if len(slice.Events) != 1 || slice.Events[0].Kind != instance.KindTurn || slice.Events[0].Ref != id {
		t.Fatalf("the room's log holds %+v; viewers reconnecting after the restart need to be told the turn ended", slice.Events)
	}
}
