package roomlive

import (
	"testing"
	"time"
)

// waitSentAway reads until the viewer is told to subscribe again, which the live layer does when a room's stream cannot be followed
// from where the viewer is.
func waitSentAway(t *testing.T, c *client) {
	t.Helper()
	for {
		p, ok := c.next(5 * time.Second)
		if !ok {
			t.Fatal("the viewer was never sent away to subscribe again")
		}
		if p.Unsubscribe != nil {
			return
		}
	}
}

func TestARoomsFeedGoesOnWhenItsLogStartsAgain(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada", "bob")
	ada, bob := r.dial("ada", "room-a"), r.dial("bob", "room-a")
	joined := r.join(ada, "room-a")
	r.join(bob, "room-a")
	r.backend.emit("room-a", "turn", "t1")
	r.backend.emit("room-a", "turn", "t2")
	ada.frames(2, 5*time.Second)
	bob.frames(2, 5*time.Second)

	if err := r.backend.log.Drop("room-a"); err != nil {
		t.Fatal(err)
	}
	r.backend.emit("room-a", "turn", "x1")
	waitSentAway(t, ada)
	waitSentAway(t, bob)

	// A node tells the broker of a room only when its first viewer comes, so a viewer that comes back while the other has not yet
	// gone would find the room with nobody following it if the feed had ended.
	if followed := r.server.broker.Following(); followed != 1 {
		t.Fatalf("the room's feed ended with its log, and %d rooms are followed", followed)
	}
	back := ada.subscribe(StreamChannel("room-a"), &position{Offset: 2, Epoch: joined.Epoch})
	if back.Err != nil || back.Recovered || back.Epoch == joined.Epoch {
		t.Fatalf("a viewer from the log before cannot be recovered and is told where the new one stands: %+v", back)
	}
	r.backend.emit("room-a", "turn", "x2")
	got := ada.frames(1, 5*time.Second)[0]
	if got.Seq != 2 || got.ID != "x2" {
		t.Fatalf("the second event of the new log is x2, and the first log's second event was drawn for it: %+v", got)
	}
}
