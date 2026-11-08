package roomlive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestEventsReachEveryViewerOfARoomInOrder(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada", "bob")
	const viewers, events = 40, 25
	var watching []*client
	for i := range viewers {
		who := "ada"
		if i%2 == 1 {
			who = "bob"
		}
		c := r.dial(who, "room-a")
		r.join(c, "room-a")
		watching = append(watching, c)
	}
	for i := range events {
		r.backend.emit("room-a", "turn", fmt.Sprintf("t%d", i))
	}
	for n, c := range watching {
		for i, f := range c.frames(events, 10*time.Second) {
			if f.Seq != uint64(i+1) || f.Type != FrameTurn || f.ID != fmt.Sprintf("t%d", i) || f.Room != "room-a" || f.V != Version {
				t.Fatalf("viewer %d frame %d: %+v", n, i, f)
			}
		}
	}
	if drawn := r.backend.draws.Load(); drawn != events {
		t.Fatalf("each event should be drawn once for all %d viewers, but %d draws were made for %d events", viewers, drawn, events)
	}
}

func TestEventsOfOtherRoomsDoNotReachAViewer(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	r.backend.addRoom("room-b", "ada")
	c := r.dial("ada", "room-a")
	r.join(c, "room-a")
	r.backend.emit("room-b", "turn", "other")
	r.backend.emit("room-a", "note", "mine")
	got := c.frames(1, 5*time.Second)
	if got[0].Room != "room-a" || got[0].ID != "mine" {
		t.Fatalf("a viewer got %+v", got[0])
	}
	if p, ok := c.next(200 * time.Millisecond); ok && p.Data != nil {
		t.Fatalf("an event of another room arrived: %s", p.Data)
	}
}

func TestAResentCommandIsDoneOnce(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	c := r.dial("ada", "room-a")
	r.join(c, "room-a")

	first, rerr := c.rpc(CmdSay, Command{ID: "c-1", Room: "room-a", Thread: "1", Text: "hello"})
	if rerr != nil || !first.OK || first.Dupe {
		t.Fatalf("first send: %+v %v", first, rerr)
	}
	again, _ := c.rpc(CmdSay, Command{ID: "c-1", Room: "room-a", Thread: "1", Text: "hello"})
	if !again.OK || !again.Dupe || again.Ref != first.Ref {
		t.Fatalf("the resend should return the first result: %+v", again)
	}

	c.close()
	back := r.dial("ada", "room-a")
	after, _ := back.rpc(CmdSay, Command{ID: "c-1", Room: "room-a", Thread: "1", Text: "hello"})
	if !after.Dupe || after.Ref != first.Ref {
		t.Fatalf("a resend after a reconnect is still the same command: %+v", after)
	}
	if did := r.backend.done(); len(did) != 1 {
		t.Fatalf("the command was carried out %d times", len(did))
	}
	other, _ := back.rpc(CmdSay, Command{ID: "c-2", Room: "room-a", Thread: "1", Text: "hello"})
	if other.Dupe || other.Ref == first.Ref {
		t.Fatalf("a different id is a different command: %+v", other)
	}
}

func TestResendsSentTogetherAreDoneOnce(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	var clients []*client
	for range 4 {
		clients = append(clients, r.dial("ada", "room-a"))
	}
	var wg sync.WaitGroup
	results := make([]Result, len(clients))
	for i, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = c.rpc(CmdSay, Command{ID: "same", Room: "room-a", Thread: "1", Text: "hi"})
		}()
	}
	wg.Wait()
	if did := r.backend.done(); len(did) != 1 {
		t.Fatalf("commands sent together under one id were carried out %d times", len(did))
	}
	for _, res := range results {
		if !res.OK || res.Ref != results[0].Ref {
			t.Fatalf("results differ: %+v", results)
		}
	}
}

func TestSomeoneNotInTheRoomIsRefusedAtEveryStep(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")

	if _, status, err := connectClient(t, r.http.URL, "/live", "", false); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated upgrade should be 401: %d %v", status, err)
	}
	if _, status, err := connectClient(t, r.http.URL, "/live?room=room-a", "mallory", false); err == nil || status != http.StatusNotFound {
		t.Fatalf("an upgrade for a room the person is not in should be 404: %d %v", status, err)
	}

	mallory, _, err := connectClient(t, r.http.URL, "/live", "mallory", false)
	if err != nil {
		t.Fatal(err)
	}
	defer mallory.close()
	if got := mallory.subscribe(StreamChannel("room-a"), nil); got.Err == nil {
		t.Fatal("subscribed to a room they are not in")
	}
	if _, rerr := mallory.rpc(CmdSay, Command{ID: "x", Room: "room-a", Text: "let me in"}); rerr == nil {
		t.Fatal("a command for a room they are not in was carried out")
	}
	if _, rerr := mallory.rpc(CmdTyping, Command{ID: "y", Room: "room-a"}); rerr == nil {
		t.Fatal("typing was passed into a room they are not in")
	}
	if len(r.backend.done()) != 0 {
		t.Fatal("the backend was asked to do something for someone who is not in the room")
	}
}

func TestAMemberTakenOutOfARoomLosesItAtOnce(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada", "bob")
	bob := r.dial("bob", "room-a")
	r.join(bob, "room-a")

	r.backend.removeMember("room-a", "bob")
	r.server.Revalidate("room-a")

	for {
		p, ok := bob.next(5 * time.Second)
		if !ok {
			t.Fatal("bob was never told he left")
		}
		if p.Unsubscribe != nil {
			break
		}
	}
	if _, rerr := bob.rpc(CmdSay, Command{ID: "z", Room: "room-a", Text: "still here"}); rerr == nil {
		t.Fatal("a person taken out of the room could still speak in it")
	}
	r.backend.emit("room-a", "turn", "t1")
	if p, ok := bob.next(300 * time.Millisecond); ok && p.Data != nil {
		t.Fatalf("bob still got the room's events: %s", p.Data)
	}
}

func TestTypingAndPresenceAreSeenInTheRoomAndNeverLogged(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada", "bob")
	ada := r.dial("ada", "room-a")
	r.join(ada, "room-a")
	bob := r.dial("bob", "room-a")
	r.join(bob, "room-a")

	var joined bool
	for !joined {
		p, ok := ada.next(5 * time.Second)
		if !ok {
			t.Fatal("ada never saw bob arrive")
		}
		joined = p.Join != nil && p.Channel == LiveChannel("room-a")
	}

	before, _ := r.backend.RoomHead("room-a")
	for range 5 {
		if res, rerr := bob.rpc(CmdTyping, Command{ID: fmt.Sprint(time.Now().UnixNano()), Room: "room-a", Thread: "1"}); rerr != nil || !res.OK {
			t.Fatalf("typing: %+v %v", res, rerr)
		}
	}
	var typing []Typing
	deadline := time.After(1 * time.Second)
collect:
	for {
		select {
		case p := <-ada.Pushes:
			var tp Typing
			if p.Channel == LiveChannel("room-a") && json.Unmarshal(p.Data, &tp) == nil && tp.Type == FrameTyping {
				typing = append(typing, tp)
			}
		case <-deadline:
			break collect
		}
	}
	if len(typing) != 1 || typing[0].Who != "bob" || typing[0].Thread != "1" {
		t.Fatalf("five typing messages in a burst should be coalesced into one: %+v", typing)
	}
	after, _ := r.backend.RoomHead("room-a")
	if after != before {
		t.Fatalf("typing moved the log from %+v to %+v", before, after)
	}
}

func TestAnotherSitesPageCannotOpenASocket(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	header := http.Header{"X-Who": {"ada"}, "Origin": {"https://evil.example"}}
	if ws, status, err := dialWS(r.http.URL, "/live?room=room-a", header); err == nil || status != http.StatusForbidden {
		if ws != nil {
			ws.Close()
		}
		t.Fatalf("a cross-site upgrade should be refused with 403, got %d %v", status, err)
	}
}

func TestASocketWhoseSessionEndedIsClosed(t *testing.T) {
	r := newRig(t, 512, Options{Recheck: 150 * time.Millisecond})
	r.backend.addRoom("room-a", "ada")
	c := r.dial("ada", "room-a")
	r.join(c, "room-a")
	r.backend.revoke("ada")
	select {
	case <-c.Closed:
	case <-time.After(15 * time.Second):
		t.Fatal("a socket outlived the session behind it")
	}
}

func TestTooManySocketsAreTurnedAway(t *testing.T) {
	r := newRig(t, 512, Options{MaxConnections: 2})
	r.backend.addRoom("room-a", "ada")
	r.dial("ada", "room-a")
	r.dial("ada", "room-a")
	eventually(t, 5*time.Second, "both sockets to be counted", func() bool { return r.server.count() == 2 })
	if _, status, err := connectClient(t, r.http.URL, "/live?room=room-a", "ada", false); err == nil || status != http.StatusServiceUnavailable {
		t.Fatalf("a socket over the bound should get 503: %d %v", status, err)
	}
}

func TestShutdownClosesEverySocketWithGoingAwayAndWaits(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	var clients []*client
	for range 12 {
		c := r.dial("ada", "room-a")
		r.join(c, "room-a")
		clients = append(clients, c)
	}
	ctx, cancel := contextWithTimeout(10 * time.Second)
	defer cancel()
	if err := r.server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if r.server.count() != 0 {
		t.Fatalf("%d sockets are still open after shutdown returned", r.server.count())
	}
	for i, c := range clients {
		select {
		case <-c.Closed:
		case <-time.After(5 * time.Second):
			t.Fatalf("client %d was not closed", i)
		}
		if code := c.CloseCode.Load(); code != 1001 {
			t.Fatalf("client %d was closed with %d, want 1001", i, code)
		}
	}
	if _, status, err := connectClient(t, r.http.URL, "/live?room=room-a", "ada", false); err == nil || status != http.StatusServiceUnavailable {
		t.Fatalf("a socket after shutdown should get 503: %d %v", status, err)
	}
}

func TestAnOversizedMessageClosesTheSocket(t *testing.T) {
	r := newRig(t, 512, Options{ReadLimit: 1024})
	r.backend.addRoom("room-a", "ada")
	c := r.dial("ada", "room-a")
	big := make([]byte, 4096)
	for i := range big {
		big[i] = 'a'
	}
	_ = c.WriteRaw(0x1, big)
	select {
	case <-c.Closed:
	case <-time.After(5 * time.Second):
		t.Fatal("a message over the read limit was accepted")
	}
}

func TestTheSocketIsPingedEveryThirtySeconds(t *testing.T) {
	r := newRig(t, 512, Options{})
	r.backend.addRoom("room-a", "ada")
	c := r.dial("ada", "room-a")
	var hello struct {
		Ping uint32 `json:"ping"`
		Pong bool   `json:"pong"`
	}
	if err := json.Unmarshal(c.Hello, &hello); err != nil {
		t.Fatal(err)
	}
	if hello.Ping != 30 {
		t.Fatalf("the server pings every %d seconds, which must stay under the 60 a proxy waits: %s", hello.Ping, c.Hello)
	}
}
