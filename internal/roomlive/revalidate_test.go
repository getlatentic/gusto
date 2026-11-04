package roomlive

import (
	"sync"
	"testing"
	"time"
)

func TestAMemberTakenOutWhileTheirSocketSubscribesLosesTheRoom(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada", "bob")
	bob := r.dial("bob", "room-a")
	var once sync.Once
	meanwhile := func(viewer Viewer, room string) {
		once.Do(func() {
			r.backend.removeMember(room, viewer.Who)
			r.server.Revalidate(room)
		})
	}
	r.backend.authorized.Store(&meanwhile)
	bob.subscribe(StreamChannel("room-a"), nil)
	r.backend.emit("room-a", "turn", "t1")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p, ok := bob.next(200 * time.Millisecond)
		if ok && p.Data != nil {
			t.Fatalf("a person taken out of the room while subscribing still got its events: %s", p.Data)
		}
	}
}
