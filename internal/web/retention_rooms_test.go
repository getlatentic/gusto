package web

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
)

// oldTurn leaves a passed turn of a room that ended days ago.
func oldTurn(t *testing.T, site *site, id, room string, days int) {
	t.Helper()
	spentJob(t, site.index.Root(), id, "", priced)
	if err := os.MkdirAll(filepath.Join(site.index.Root(), id, "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	ended := time.Now().AddDate(0, 0, -days).Unix()
	if err := site.index.Add(jobs.Record{ID: id, State: jobs.Passed, Brief: "a turn", Harness: "fixture", Instance: room,
		Asked: "amy@acme.test", Queued: ended, Started: ended, Ended: ended}); err != nil {
		t.Fatal(err)
	}
}

func TestATurnOfARoomThatWasRemovedIsSweptAndOneOfAKeptRoomIsNot(t *testing.T) {
	here, _, _ := keeping(t)
	room, err := here.server.instances.StartRoom("Launch", alice)
	if err != nil {
		t.Fatal(err)
	}
	oldTurn(t, here, "kept-room-turn", room.ID, 200)
	oldTurn(t, here, "gone-room-turn", "a-room-since-removed", 200)
	here.server.Sweep(time.Now())
	if _, kept := here.index.Get("kept-room-turn"); !kept {
		t.Error("a turn of a room that is kept was swept")
	}
	if _, kept := here.index.Get("gone-room-turn"); kept {
		t.Error("a turn of a room that was removed is never swept")
	}
}

func TestAHoldOnATaskThatIsGoneIsEnded(t *testing.T) {
	here, _, dir := keeping(t)
	oldJob(t, here, "held-then-gone", 1)
	if err := here.server.retention.holds.HoldJob("held-then-gone", alice, "an audit"); err != nil {
		t.Fatal(err)
	}
	if err := here.index.Remove("held-then-gone"); err != nil {
		t.Fatal(err)
	}
	here.server.Sweep(time.Now())
	held, err := here.server.retention.holds.Read()
	if err != nil {
		t.Fatal(err)
	}
	if _, still := held.Jobs["held-then-gone"]; still {
		t.Errorf("a hold on a task that is gone stays for ever in %s", dir)
	}
}
