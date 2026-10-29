package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/roomlive/livetest"
	"github.com/getlatentic/gusto/pkg/spend"
)

func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// liveRoom is a room alice made, with bob in it and the agents named, served over a real listener with the live layer on. polls
// counts the requests the old polling makes, which a live page must not.
type liveRoom struct {
	t      *testing.T
	site   *site
	id     string
	thread string
	http   *httptest.Server
	polls  atomic.Int64
}

func newLiveRoom(t *testing.T, work *roomWork, agents ...[2]string) *liveRoom {
	t.Helper()
	site, id, thread := aRoom(t, work, agents...)
	// A running server has the proxy's ledger in memory; without one every page rebuilds it from every job on disk.
	ledger, err := spend.Rebuild(site.index.Root(), spend.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	site.server.UseLedger(ledger)
	if err := site.server.UseLive(roomlive.Options{Recheck: 2 * time.Second}); err != nil {
		t.Fatal(err)
	}
	site.handler = site.server.Handler()
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}})
	l := &liveRoom{t: t, site: site, id: id, thread: thread}
	l.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/version") || strings.HasSuffix(r.URL.Path, "/turns") {
			l.polls.Add(1)
		}
		site.handler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		l.http.CloseClientConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = site.server.CloseLive(ctx)
		l.http.Close()
	})
	return l
}

func (l *liveRoom) header(who string) http.Header {
	cookie := l.site.as(l.t, who).cookie
	return http.Header{"Cookie": {cookie.Name + "=" + cookie.Value}}
}

// socket opens a person's socket to the room and subscribes to its stream and live channel from where the stream stands.
func (l *liveRoom) socket(who string) *livetest.Client {
	l.t.Helper()
	c, status, err := livetest.Connect(l.t, l.http.URL, "/live?room="+l.id, l.header(who), false)
	if err != nil {
		l.t.Fatalf("%s could not open a socket: %v (%d)", who, err, status)
	}
	l.t.Cleanup(c.Close)
	for _, channel := range []string{roomlive.StreamChannel(l.id), roomlive.LiveChannel(l.id)} {
		if got := c.Subscribe(channel, nil); got.Err != nil {
			l.t.Fatalf("%s could not subscribe to %s: %s", who, channel, got.Err.Message)
		}
	}
	return c
}

func (l *liveRoom) threadID() string { return l.site.server.instances.Latest(l.id).ID }

// say sends a message over a socket.
func (l *liveRoom) say(c *livetest.Client, id, text string, retry bool) roomlive.Result {
	l.t.Helper()
	data, rerr := c.RPC(roomlive.CmdSay, roomlive.Command{V: 1, ID: id, Room: l.id, Thread: l.threadID(), Text: text, Retry: retry})
	if rerr != nil {
		l.t.Fatalf("say: %s", rerr.Message)
	}
	var result roomlive.Result
	_ = json.Unmarshal(data, &result)
	return result
}

// frames reads the stream's frames from a socket until it has count of them, or fails.
func frames(t *testing.T, c *livetest.Client, count int) []roomlive.Frame {
	t.Helper()
	var got []roomlive.Frame
	deadline := time.Now().Add(10 * time.Second)
	for len(got) < count {
		p, ok := c.Next(time.Until(deadline))
		if !ok {
			t.Fatalf("wanted %d frames, got %d: %+v", count, len(got), got)
		}
		var f roomlive.Frame
		if strings.HasPrefix(p.Channel, "room:") && json.Unmarshal(p.Data, &f) == nil {
			got = append(got, f)
		}
	}
	return got
}

func TestAMessageAndAnAgentsReplyAppearInEveryOpenViewWithoutPolling(t *testing.T) {
	work := &roomWork{replies: map[string]string{"a1": "the reply from Scout"}}
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	ada, ben := l.socket(alice), l.socket(bob)

	if result := l.say(ada, "c-1", "what is next?", false); !result.OK {
		t.Fatalf("saying it over the socket: %+v", result)
	}
	// A message makes a turn that is queued, running, and finished: each is told, and the last carries the reply.
	for name, c := range map[string]*livetest.Client{"alice": ada, "bob": ben} {
		seen := frames(t, c, 1)
		if seen[0].Type != roomlive.FrameTurn || !strings.Contains(seen[0].HTML, "what is next?") {
			t.Fatalf("%s first saw %+v", name, seen[0])
		}
		for !strings.Contains(seen[len(seen)-1].HTML, "the reply from Scout") {
			seen = append(seen, frames(t, c, 1)...)
		}
		for i := 1; i < len(seen); i++ {
			if seen[i].Seq <= seen[i-1].Seq {
				t.Fatalf("%s saw events out of order: %+v", name, seen)
			}
		}
	}
	if l.polls.Load() != 0 {
		t.Fatalf("the views were updated by %d polling requests", l.polls.Load())
	}
}

func TestANoteAppearsInEveryOpenView(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	ada, ben := l.socket(alice), l.socket(bob)
	if result := l.say(ben, "c-note", "lunch at noon", false); !result.OK {
		t.Fatalf("a note: %+v", result)
	}
	for name, c := range map[string]*livetest.Client{"alice": ada, "bob": ben} {
		got := frames(t, c, 1)
		if got[0].Type != roomlive.FrameNote || !strings.Contains(got[0].HTML, "lunch at noon") || got[0].ID != "n1" {
			t.Fatalf("%s saw %+v", name, got[0])
		}
	}
}

func TestADrawnTurnIsTheSameHTMLTheRoomPageShows(t *testing.T) {
	work := &roomWork{replies: map[string]string{"a1": "the reply from Scout"}}
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	ada := l.socket(alice)
	l.say(ada, "c-1", "what is next?", false)
	work.waitTurns(t, l.site, 1)

	var last roomlive.Frame
	for !strings.Contains(last.HTML, "the reply from Scout") {
		last = frames(t, ada, 1)[0]
	}
	// Bob did not say it, so the page is drawn for him as the live update is drawn for everybody.
	page := l.site.as(t, bob).ask(t, "GET", l.thread+"/turns", nil).Body.String()
	article := page[strings.Index(page, `<article id="`+last.ID+`"`):]
	article = article[:strings.Index(article, "</article>")+len("</article>")]
	if article != strings.TrimSpace(last.HTML) {
		t.Fatalf("the live update and the page differ:\nlive: %s\npage: %s", last.HTML, article)
	}
}

func TestAResentMessageIsOneTurnEvenAfterARestart(t *testing.T) {
	work := &roomWork{}
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	ada := l.socket(alice)
	first := l.say(ada, "c-1", "do it once", false)
	again := l.say(ada, "c-1", "do it once", false)
	if !first.OK || !again.Dupe || again.Ref != first.Ref {
		t.Fatalf("a resend is answered with the first: %+v then %+v", first, again)
	}
	work.waitTurns(t, l.site, 1)

	// The server restarts: what it remembered of commands is gone, and the browser, which never heard the answer, sends it again.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.site.server.CloseLive(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.site.server.UseLive(roomlive.Options{}); err != nil {
		t.Fatal(err)
	}
	l.site.handler = l.site.server.Handler()
	back := l.socket(alice)
	resent := l.say(back, "c-1", "do it once", true)
	if !resent.OK || resent.Ref != first.Ref {
		t.Fatalf("the resend after a restart should find the turn it made: %+v", resent)
	}
	time.Sleep(200 * time.Millisecond)
	if got := len(l.site.index.List()); got != 1 {
		t.Fatalf("saying it again after a restart made %d turns", got)
	}
}

func TestSomeoneNotInTheRoomCannotOpenItsSocketOrSpeakInIt(t *testing.T) {
	work := &roomWork{}
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	carol := "carol@example.com"

	if _, status, err := livetest.Connect(t, l.http.URL, "/live?room="+l.id, http.Header{}, false); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("a stranger's socket: %d %v", status, err)
	}
	if _, status, err := livetest.Connect(t, l.http.URL, "/live?room="+l.id, l.header(carol), false); err == nil || status != http.StatusNotFound {
		t.Fatalf("a socket for a room the person is not in: %d %v", status, err)
	}
	c, _, err := livetest.Connect(t, l.http.URL, "/live", l.header(carol), false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if got := c.Subscribe(roomlive.StreamChannel(l.id), nil); got.Err == nil {
		t.Fatal("carol subscribed to a room she is not in")
	}
	if _, rerr := c.RPC(roomlive.CmdSay, roomlive.Command{V: 1, ID: "x", Room: l.id, Thread: l.threadID(), Text: "hello"}); rerr == nil {
		t.Fatal("carol spoke in a room she is not in")
	}
	if len(l.site.index.List()) != 0 {
		t.Fatal("a turn was made for somebody who is not in the room")
	}
}

func TestAMemberTakenOutOfTheRoomIsTakenOffItsSocket(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	ben := l.socket(bob)
	if removed := l.site.as(t, alice).ask(t, "POST", "/rooms/"+l.id+"/members/remove", url.Values{"who": {bob}}); removed.Code != http.StatusSeeOther {
		t.Fatalf("removing bob gave %d", removed.Code)
	}
	for {
		p, ok := ben.Next(10 * time.Second)
		if !ok {
			t.Fatal("bob was never taken off the room's stream")
		}
		if p.Unsubscribe != nil {
			break
		}
	}
	if _, rerr := ben.RPC(roomlive.CmdSay, roomlive.Command{V: 1, ID: "late", Room: l.id, Thread: l.threadID(), Text: "still here?"}); rerr == nil {
		t.Fatal("bob could speak in a room he was taken out of")
	}
}

func TestAnAgentIsStoppedFromTheSocket(t *testing.T) {
	work := &roomWork{hold: make(chan struct{})}
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	ada := l.socket(alice)
	made := l.say(ada, "c-1", "take a long time", false)
	eventually(t, 5*time.Second, "the turn to start", func() bool {
		record, found := l.site.index.Get(made.Ref)
		return found && record.State == jobs.Running
	})
	data, rerr := ada.RPC(roomlive.CmdStop, roomlive.Command{V: 1, ID: "s-1", Room: l.id, Ref: made.Ref})
	if rerr != nil {
		t.Fatal(rerr.Message)
	}
	var result roomlive.Result
	_ = json.Unmarshal(data, &result)
	if !result.OK {
		t.Fatalf("stopping: %+v", result)
	}
	eventually(t, 5*time.Second, "the turn to end", func() bool {
		record, _ := l.site.index.Get(made.Ref)
		return record.State.Done()
	})
	var last roomlive.Frame
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(last.HTML, "Stopped by") && time.Now().Before(deadline) {
		if p, ok := ada.Next(time.Second); ok {
			_ = json.Unmarshal(p.Data, &last)
		}
	}
	if !strings.Contains(last.HTML, "Stopped by "+alice) {
		t.Fatalf("the room was not told the turn was stopped: %+v", last)
	}
}

func TestTypingInARoomIsShownToTheOthersAndNotKept(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	ada, ben := l.socket(alice), l.socket(bob)
	before, _ := l.site.server.instances.RoomHead(l.id)
	if _, rerr := ben.RPC(roomlive.CmdTyping, roomlive.Command{V: 1, ID: "t-1", Room: l.id, Thread: l.threadID()}); rerr != nil {
		t.Fatal(rerr.Message)
	}
	for {
		p, ok := ada.Next(5 * time.Second)
		if !ok {
			t.Fatal("alice never saw bob typing")
		}
		var typing roomlive.Typing
		if p.Channel == roomlive.LiveChannel(l.id) && json.Unmarshal(p.Data, &typing) == nil && typing.Type == roomlive.FrameTyping {
			if typing.Who != bob || typing.Name != "Bob" {
				t.Fatalf("typing from %+v", typing)
			}
			break
		}
	}
	if after, _ := l.site.server.instances.RoomHead(l.id); after != before {
		t.Fatalf("typing moved the room's log from %+v to %+v", before, after)
	}
}

func TestTheRoomPageSaysWhereItsStreamStandsAndLoadsTheSocketScripts(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	mine := l.site.as(t, alice)
	mine.ask(t, "POST", l.thread+"/say", url.Values{"said": {"hello"}})
	head, _ := l.site.server.instances.RoomHead(l.id)
	page := mine.ask(t, "GET", l.thread, nil).Body.String()
	for _, want := range []string{
		`data-live="/live?room=` + l.id + `"`, `data-epoch="` + head.Epoch + `"`, `data-room="` + l.id + `"`,
		`/static/centrifuge.js?v=`, `/static/live.js?v=`, `id="typing"`, `id="here"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the room's page lacks %s", want)
		}
	}
	if want := `data-seq="` + strconv.FormatUint(head.Seq, 10) + `"`; head.Seq == 0 || !strings.Contains(page, want) {
		t.Errorf("the page should follow from the newest event, %s: %s", want, page[strings.Index(page, `id="turns"`):][:200])
	}
}

func TestSocketsAreClosedWithGoingAwayWhenTheServerShutsDown(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	ada := l.socket(alice)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := l.site.server.CloseLive(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ada.Closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the socket was not closed")
	}
	if code := ada.CloseCode.Load(); code != 1001 {
		t.Fatalf("closed with %d, want 1001", code)
	}
}
