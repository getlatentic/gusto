package roomlive

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// What a viewer is sent when it comes back, and what the live layer keeps true across reconnects, restarts and a viewer that
// cannot keep up.

func TestAReconnectingViewerIsReplayedExactlyWhatItMissed(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	first := r.dial("ada", "room-a")
	joined := r.join(first, "room-a")
	r.backend.emit("room-a", "turn", "t1")
	r.backend.emit("room-a", "turn", "t2")
	seen := first.frames(2, 5*time.Second)
	position := position{Offset: seen[1].Seq, Epoch: joined.Epoch}
	first.close()

	r.backend.emit("room-a", "turn", "t3")
	r.backend.emit("room-a", "note", "n1")
	r.backend.emit("room-a", "turn", "t4")

	second := r.dial("ada", "room-a")
	got := second.subscribe(StreamChannel("room-a"), &position)
	if got.Err != nil || !got.Recovered {
		t.Fatalf("the viewer should recover: %+v", got)
	}
	var ids []string
	for _, pub := range got.Pubs {
		var f Frame
		_ = json.Unmarshal(pub.Data, &f)
		ids = append(ids, fmt.Sprintf("%d:%s", f.Seq, f.ID))
	}
	if fmt.Sprint(ids) != "[3:t3 4:n1 5:t4]" {
		t.Fatalf("replayed %v", ids)
	}
	r.backend.emit("room-a", "turn", "t5")
	if live := second.frames(1, 5*time.Second); live[0].Seq != 6 || live[0].ID != "t5" {
		t.Fatalf("after the replay the next event is live: %+v", live[0])
	}
}

func TestReplayOfAnEventChangedAgainIsAMarkerAndTheFinalState(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	probe := r.dial("ada", "room-a")
	joined := r.join(probe, "room-a")
	probe.close()
	for range 3 {
		r.backend.emit("room-a", "turn", "t1")
	}
	r.backend.emit("room-a", "turn", "t2")

	second := r.dial("ada", "room-a")
	got := second.subscribe(StreamChannel("room-a"), &position{Offset: 0, Epoch: joined.Epoch})
	var types []string
	for _, pub := range got.Pubs {
		var f Frame
		_ = json.Unmarshal(pub.Data, &f)
		types = append(types, f.Type)
	}
	if fmt.Sprint(types) != "[skip skip turn turn]" {
		t.Fatalf("replay frames %v; a turn changed three times should be one page and two markers", types)
	}
}

func TestAViewerBeyondTheWindowIsToldToStartAgain(t *testing.T) {
	r := newRig(t, 8, Options{})
	r.backend.addRoom("room-a", "ada")
	probe := r.dial("ada", "room-a")
	joined := r.join(probe, "room-a")
	probe.close()
	for i := range 40 {
		r.backend.emit("room-a", "turn", fmt.Sprintf("t%d", i))
	}
	second := r.dial("ada", "room-a")
	got := second.subscribe(StreamChannel("room-a"), &position{Offset: 1, Epoch: joined.Epoch})
	if got.Err != nil || got.Recovered || len(got.Pubs) != 0 {
		t.Fatalf("a cursor from before the window cannot be recovered: %+v", got)
	}
	if got.Offset != 40 {
		t.Fatalf("it is told where the stream stands: %d", got.Offset)
	}
}

func TestAViewerFromAnotherEpochCannotRecover(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	r.backend.emit("room-a", "turn", "t1")
	c := r.dial("ada", "room-a")
	got := c.subscribe(StreamChannel("room-a"), &position{Offset: 1, Epoch: "not-this-stream"})
	if got.Err != nil || got.Recovered {
		t.Fatalf("a position from another epoch is not recoverable: %+v", got)
	}
}

func TestSequencesAndReplaySurviveAServerRestart(t *testing.T) {
	dir := t.TempDir()
	first := newRigOn(t, newFakeBackend(dir, 512), Options{})
	first.backend.addRoom("room-a", "ada")
	c := first.dial("ada", "room-a")
	joined := first.join(c, "room-a")
	first.backend.emit("room-a", "turn", "t1")
	first.backend.emit("room-a", "turn", "t2")
	seen := c.frames(2, 5*time.Second)
	position := position{Offset: seen[1].Seq, Epoch: joined.Epoch}
	first.stop()

	second := newRigOn(t, newFakeBackend(dir, 512), Options{})
	second.backend.addRoom("room-a", "ada")
	second.backend.emit("room-a", "turn", "t3")
	again := second.dial("ada", "room-a")
	got := again.subscribe(StreamChannel("room-a"), &position)
	if got.Err != nil || !got.Recovered || len(got.Pubs) != 1 {
		t.Fatalf("after a restart the viewer should be replayed what it missed: %+v", got)
	}
	var f Frame
	_ = json.Unmarshal(got.Pubs[0].Data, &f)
	if f.Seq != 3 || f.ID != "t3" || got.Epoch != joined.Epoch {
		t.Fatalf("the sequence and epoch carry on across the restart: %+v epoch %q", f, got.Epoch)
	}
}

func TestACommitWhileTheLastViewerLeavesIsNotLostOrShownTwice(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	hold := make(chan struct{})
	r.backend.gate.Store(&hold)

	gone := r.dial("ada", "room-a")
	joined := r.join(gone, "room-a")
	r.backend.emit("room-a", "turn", "t1")
	eventually(t, 5*time.Second, "the event to be being drawn", func() bool { return r.backend.draws.Load() >= 1 })
	gone.close()
	eventually(t, 5*time.Second, "the room's feed to stop with its last viewer", func() bool { return r.server.broker.Following() == 0 })
	close(hold)

	back := r.dial("ada", "room-a")
	got := back.subscribe(StreamChannel("room-a"), &position{Offset: 0, Epoch: joined.Epoch})
	if !got.Recovered || len(got.Pubs) != 1 {
		t.Fatalf("the event that was in flight should be replayed once: %+v", got)
	}
	r.backend.emit("room-a", "turn", "t2")
	next := back.frames(1, 5*time.Second)
	if next[0].Seq != 2 {
		t.Fatalf("the next live event is 2, got %+v", next[0])
	}
	if p, ok := back.next(200 * time.Millisecond); ok && p.Data != nil {
		t.Fatalf("the in-flight event was shown again: %s", p.Data)
	}
}

func TestConcurrentCommitsReachViewersOnceAndInOrder(t *testing.T) {
	r := newRig(t, 4096, Options{})
	r.backend.addRoom("room-a", "ada")
	var watching []*client
	for range 8 {
		c := r.dial("ada", "room-a")
		r.join(c, "room-a")
		watching = append(watching, c)
	}
	const writers, each = 10, 30
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				r.backend.emit("room-a", "turn", fmt.Sprintf("w%d-%d", w, i))
			}
		}()
	}
	wg.Wait()
	for n, c := range watching {
		for i, f := range c.frames(writers*each, 20*time.Second) {
			if f.Seq != uint64(i+1) {
				t.Fatalf("viewer %d got sequence %d at place %d", n, f.Seq, i)
			}
		}
	}
}

func TestASlowViewerIsDisconnectedAndTheOthersAreNotDelayed(t *testing.T) {
	r := newRig(t, 512, Options{QueueBytes: 1 << 20, WriteTimeout: time.Second})
	r.backend.bulk = 200 << 10
	r.backend.addRoom("room-a", "ada")

	slow, _, err := connectClient(t, r.http.URL, "/live?room=room-a", "ada", false)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.close()
	joined := r.join(slow, "room-a")
	fast := r.dial("ada", "room-a")
	r.join(fast, "room-a")
	slow.Stall(true)

	started := time.Now()
	const events = 60
	for i := range events {
		r.backend.emit("room-a", "turn", fmt.Sprintf("t%d", i))
		if got := fast.frames(1, 10*time.Second); got[0].Seq != uint64(i+1) {
			t.Fatalf("the fast viewer got sequence %d, want %d", got[0].Seq, i+1)
		}
	}
	if took := time.Since(started); took > 15*time.Second {
		t.Fatalf("a stalled viewer delayed the others: %v", took)
	}
	slow.Stall(false)
	select {
	case <-slow.Closed:
	case <-time.After(15 * time.Second):
		t.Fatal("the stalled viewer was never disconnected")
	}

	back := r.dial("ada", "room-a")
	recovered := back.subscribe(StreamChannel("room-a"), &position{Offset: 0, Epoch: joined.Epoch})
	if recovered.Err != nil {
		t.Fatal(recovered.Err.Message)
	}
	if recovered.Recovered || len(recovered.Pubs) != 0 {
		t.Fatalf("twelve megabytes cannot be replayed to a socket that holds one: %+v", recovered)
	}
	if recovered.Offset != events {
		t.Fatalf("a viewer too far behind is told where the stream stands: %d", recovered.Offset)
	}
}

func TestAReplayTooHeavyToSendIsASnapshotAndNotADisconnectionOverAndOver(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.bulk = 200 << 10
	r.backend.addRoom("room-a", "ada")
	probe := r.dial("ada", "room-a")
	joined := r.join(probe, "room-a")
	probe.close()
	for i := range 6 {
		r.backend.emit("room-a", "turn", fmt.Sprintf("t%d", i))
	}

	back := r.dial("ada", "room-a")
	got := back.subscribe(StreamChannel("room-a"), &position{Offset: 0, Epoch: joined.Epoch})
	if got.Err != nil || got.Recovered || got.Offset != 6 {
		t.Fatalf("a replay over its byte budget is not attempted: %+v", got)
	}
	// The socket is still there, and is live from where the stream stands.
	r.backend.emit("room-a", "turn", "t6")
	if next := back.frames(1, 5*time.Second); next[0].Seq != 7 {
		t.Fatalf("the socket should carry on live: %+v", next[0])
	}
}
