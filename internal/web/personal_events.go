package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/getlatentic/haas/internal/jobs"
)

// eventEvery is how often the stream looks at a person's jobs, and keepAlive how long it may be quiet before it sends a
// comment, which keeps a connection a proxy would drop for being idle.
var (
	eventEvery = time.Second
	keepAlive  = 20 * time.Second
)

// turnEvents name what happened to one of the person's jobs, in the order jobs go through.
var turnEvents = map[jobs.State]string{
	jobs.Queued:      "turn.queued",
	jobs.Running:     "turn.started",
	jobs.Passed:      "turn.finished",
	jobs.Failed:      "turn.failed",
	jobs.Interrupted: "turn.interrupted",
}

// turnEvent is what a tray or a notification needs to name a job: which it is, where it was said, and how it stands.
type turnEvent struct {
	Job      string `json:"job"`
	Instance string `json:"instance,omitempty"`
	Thread   string `json:"thread,omitempty"`
	Brief    string `json:"brief"`
	State    string `json:"state"`
	At       int64  `json:"at"`
}

// eventsJSON streams what happens to a person's own jobs as server-sent events, so that a tray can say when an agent
// finishes without anybody polling the pages.
//
// A connection first hears the state of every job that has not ended, and of every job that ended after ?since=<unix
// seconds> when a client names one, which is how it catches up after it was away. It then hears each change. An event
// `ready` marks the end of that first part.
func (s *Server) eventsJSON(w http.ResponseWriter, r *http.Request) {
	if _, streams := w.(http.Flusher); !streams {
		http.Error(w, "this connection cannot stream", http.StatusInternalServerError)
		return
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	stream := &feed{to: w, control: http.NewResponseController(w)}
	stream.flush()

	told := map[string]jobs.State{}
	ticker := time.NewTicker(eventEvery)
	defer ticker.Stop()
	quiet := time.Now()
	for first := true; ; first = false {
		records := s.ownJobs(r, s.index.List())
		for i := len(records) - 1; i >= 0; i-- {
			record := records[i]
			if state, known := told[record.ID]; known && state == record.State {
				continue
			}
			_, known := told[record.ID]
			told[record.ID] = record.State
			if first && !known && record.State.Done() && (since == 0 || record.Ended < since) {
				continue
			}
			stream.event(record)
			quiet = time.Now()
		}
		if first {
			stream.send("event: ready\ndata: {}\n\n")
		} else if time.Since(quiet) > keepAlive {
			stream.send(": keep-alive\n\n")
			quiet = time.Now()
		}
		stream.flush()
		if stream.gone {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (f *feed) event(record jobs.Record) {
	at := record.Ended
	switch {
	case at == 0 && record.Started != 0:
		at = record.Started
	case at == 0:
		at = record.Queued
	}
	body, err := json.Marshal(turnEvent{Job: record.ID, Instance: record.Instance, Thread: record.Thread, Brief: record.Brief,
		State: string(record.State), At: at})
	if err != nil {
		return
	}
	f.send("id: %s:%s\nevent: %s\ndata: %s\n\n", record.ID, record.State, turnEvents[record.State], body)
}
