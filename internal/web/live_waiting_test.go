package web

import (
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/roomlive/livetest"
	"github.com/getlatentic/gusto/internal/tasks"
)

// A turn that waits for its agent says so in every open view as it is queued, and is told again when it starts and ends.
func TestATurnWaitingForItsAgentIsShownWaitingLiveAndThenDone(t *testing.T) {
	work := &roomWork{hold: make(chan struct{})}
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	ada, ben := l.socket(alice), l.socket(bob)

	first := l.say(ada, "c-1", "first question", false)
	eventually(t, 5*time.Second, "the first turn to start", func() bool {
		record, _ := l.site.index.Get(first.Ref)
		return record.State == tasks.Running
	})
	second := l.say(ada, "c-2", "second question", false)

	for name, c := range map[string]*livetest.Client{"alice": ada, "bob": ben} {
		var waiting roomlive.Frame
		for !strings.Contains(turnOf(t, waiting).Holding, "Waiting for Scout") || waiting.ID != second.Ref {
			waiting = frames(t, c, 1)[0]
		}
		if turn := turnOf(t, waiting); turn.Done || turn.Started != 0 {
			t.Fatalf("%s was shown a turn that waits as working: %+v", name, turn)
		}
	}

	close(work.hold)
	for name, c := range map[string]*livetest.Client{"alice": ada, "bob": ben} {
		var last roomlive.Frame
		for !(last.ID == second.Ref && turnOf(t, last).Done) {
			last = frames(t, c, 1)[0]
		}
		if turn := turnOf(t, last); turn.Holding != "" || !strings.Contains(turn.ReplyHTML, "done") {
			t.Fatalf("%s saw the finished turn as %+v", name, turn)
		}
	}
}

// What a turn behind another waits for changes when the one ahead starts or ends, with no state change of its own, so it is told.
func TestTurnsQueuedBehindAreToldAgainWhenOneAheadChanges(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	a := l.site.server.announce
	a.note(tasks.Record{ID: "behind", Instance: l.id, Thread: "1", State: tasks.Queued})
	a.note(tasks.Record{ID: "ahead", Instance: l.id, Thread: "1", State: tasks.Running})
	eventually(t, 5*time.Second, "both turns to be announced", func() bool {
		slice, _ := l.site.server.instances.RoomEvents(l.id, 0, 0)
		seen := map[string]bool{}
		for _, event := range slice.Events {
			seen[event.Ref] = true
		}
		return seen["ahead"] && seen["behind"]
	})
}

// The room's notice that two turns changed one file is a note, and reaches every open view as the others do, drawn as a notice.
func TestAMergeNoticeAppearsInEveryOpenView(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	ada, ben := l.socket(alice), l.socket(bob)
	if _, err := l.site.server.instances.Notice(l.id, l.threadID(), "Scout and Draft both changed plan.md. Scout's version is kept."); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*livetest.Client{"alice": ada, "bob": ben} {
		got := frames(t, c, 1)[0]
		if notice := turnOf(t, got); got.Type != roomlive.FrameNote || !strings.Contains(notice.Said, "both changed plan.md") || notice.Kind != "notice" {
			t.Fatalf("%s saw %+v", name, got)
		}
	}
}
