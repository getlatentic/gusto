package web

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/tasks"
)

// A page that was told something it lost, or slept through, finds out by comparing the version of what it shows with the server's.
// The two are the same words only when the page shows the room as it is.
func TestAPageCanTellFromTheVersionOfWhatItWasShownThatTheRoomMovedOn(t *testing.T) {
	work := &roomWork{hold: make(chan struct{}), replies: map[string]string{"a1": "the reply"}}
	var release sync.Once
	letGo := func() { release.Do(func() { close(work.hold) }) }
	t.Cleanup(letGo)
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	ada := l.socket(alice)
	said := l.say(ada, "c-1", "what is next?", false)
	eventually(t, 5*time.Second, "the turn to start", func() bool {
		record, _ := l.site.index.Get(said.Ref)
		return record.State == tasks.Running
	})
	versionNow := func() string { return l.site.as(t, alice).ask(t, "GET", l.thread+"/version", nil).Body.String() }

	// The turn is drawn queued, then running; the frame that matches the room as it is now is the running one.
	var running roomlive.Frame
	for running.Ver == "" || running.Ver != versionNow() || !strings.Contains(running.HTML, "data-pending") {
		running = frames(t, ada, 1)[0]
	}

	letGo()
	var done roomlive.Frame
	for !(done.ID == said.Ref && !strings.Contains(done.HTML, "data-pending") && strings.Contains(done.HTML, "the reply")) {
		done = frames(t, ada, 1)[0]
	}
	if done.Ver == running.Ver {
		t.Fatalf("a turn that ended left the room's version as it was: %q", done.Ver)
	}
	eventually(t, 5*time.Second, "the room to be quiet", func() bool { return l.site.allDone() })
	asked := l.site.as(t, alice).ask(t, "GET", l.thread+"/turns", nil)
	if got := asked.Header().Get("X-Room-Version"); got != versionNow() {
		t.Fatalf("the conversation says it was read at %q and the room's version is %q", got, versionNow())
	}
}
