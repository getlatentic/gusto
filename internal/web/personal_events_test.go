package web

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
)

// listening is a person's connection to the event stream, read a frame at a time.
type listening struct {
	t      *testing.T
	frames chan string
	body   *http.Response
}

func listen(t *testing.T, here *tokened, secret, query string) *listening {
	t.Helper()
	server := httptest.NewServer(here.handler)
	t.Cleanup(server.Close)
	request, _ := http.NewRequest("GET", server.URL+"/api/events"+query, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the stream answered %d", response.StatusCode)
	}
	heard := &listening{t: t, frames: make(chan string, 64), body: response}
	go func() {
		var frame []string
		lines := bufio.NewScanner(response.Body)
		for lines.Scan() {
			if lines.Text() == "" {
				if len(frame) > 0 {
					heard.frames <- strings.Join(frame, "\n")
				}
				frame = nil
				continue
			}
			frame = append(frame, lines.Text())
		}
		close(heard.frames)
	}()
	return heard
}

// next is the next frame, or a failure when none comes.
func (l *listening) next() string {
	l.t.Helper()
	select {
	case frame, open := <-l.frames:
		if !open {
			l.t.Fatal("the stream ended")
		}
		return frame
	case <-time.After(5 * time.Second):
		l.t.Fatal("no event came")
		return ""
	}
}

func (l *listening) quiet(wait time.Duration) {
	l.t.Helper()
	select {
	case frame := <-l.frames:
		l.t.Errorf("an event came that should not have: %s", frame)
	case <-time.After(wait):
	}
}

// addJob puts a job in the index the way a submission does: a directory is reserved for it, and its record is kept there.
func addTurnJob(t *testing.T, here *tokened, record jobs.Record) string {
	t.Helper()
	id, err := here.server.index.Reserve(record.Brief, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	record.ID = id
	if err := here.server.index.Add(record); err != nil {
		t.Fatal(err)
	}
	return id
}

func quickEvents(t *testing.T) {
	was := eventEvery
	eventEvery = 20 * time.Millisecond
	t.Cleanup(func() { eventEvery = was })
}

func TestAPersonHearsWhenTheirOwnJobsStartAndEndAndNobodyElses(t *testing.T) {
	quickEvents(t)
	here := withTokens(t)
	index := here.server.index
	now := time.Now().Unix()
	hers := addTurnJob(t, here, jobs.Record{State: jobs.Queued, Brief: "fix the build", Harness: "opencode", Asked: alice, Instance: "i1", Thread: "t1", Queued: now})
	his := addTurnJob(t, here, jobs.Record{State: jobs.Queued, Brief: "bobs work", Harness: "opencode", Asked: bob, Queued: now})
	heard := listen(t, here, signInADevice(t, here, alice), "")

	first := heard.next()
	if !strings.Contains(first, "event: turn.queued") || !strings.Contains(first, `"job":"`+hers+`"`) || !strings.Contains(first, `"instance":"i1"`) {
		t.Fatalf("the first thing heard was %q", first)
	}
	if ready := heard.next(); !strings.Contains(ready, "event: ready") {
		t.Fatalf("the end of the catch-up was not marked: %q", ready)
	}
	if err := index.Finish(hers, jobs.Passed, 0); err != nil {
		t.Fatal(err)
	}
	if done := heard.next(); !strings.Contains(done, "event: turn.finished") || !strings.Contains(done, `"state":"passed"`) {
		t.Fatalf("a finished job was told as %q", done)
	}
	if err := index.Finish(his, jobs.Failed, 1); err != nil {
		t.Fatal(err)
	}
	heard.quiet(300 * time.Millisecond)
}

func TestACatchUpTellsWhatEndedSinceAndNotWhatEndedBefore(t *testing.T) {
	quickEvents(t)
	here := withTokens(t)
	now := time.Now().Unix()
	addTurnJob(t, here, jobs.Record{State: jobs.Passed, Brief: "long ago", Harness: "h", Asked: alice, Queued: now - 7200, Started: now - 7200, Ended: now - 7000})
	recent := addTurnJob(t, here, jobs.Record{State: jobs.Failed, Brief: "while away", Harness: "h", Asked: alice, Queued: now - 600, Started: now - 600, Ended: now - 500})
	secret := signInADevice(t, here, alice)

	heard := listen(t, here, secret, "")
	if frame := heard.next(); !strings.Contains(frame, "event: ready") {
		t.Errorf("with no ?since, finished jobs were told: %q", frame)
	}
	caught := listen(t, here, secret, "?since="+strconv.FormatInt(now-1000, 10))
	if frame := caught.next(); !strings.Contains(frame, `"job":"`+recent+`"`) || !strings.Contains(frame, "event: turn.failed") {
		t.Fatalf("the job that ended while away was told as %q", frame)
	}
	if frame := caught.next(); !strings.Contains(frame, "event: ready") {
		t.Errorf("an old job was told, or the catch-up was not marked: %q", frame)
	}
}

func TestOnlyAPersonsTokenOpensTheEventStream(t *testing.T) {
	here := withTokens(t)
	for name, secret := range here.secrets {
		if got := here.getAs(secret, "/api/events"); got.Code == http.StatusOK {
			t.Errorf("the organisation's %q token opened a person's stream", name)
		}
	}
	if got := here.getAs("", "/api/events"); got.Code != http.StatusUnauthorized {
		t.Errorf("no credential answered %d", got.Code)
	}
}
