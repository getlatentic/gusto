package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
)

// engineJSON is an agent a person may start and the models they may start it with.
type engineJSON struct {
	Name   string   `json:"name"`
	Title  string   `json:"title"`
	What   string   `json:"what"`
	Ready  bool     `json:"ready"`
	Why    string   `json:"why,omitempty"`
	Models []string `json:"models"`
}

// enginesJSON lists what this deployment can run and what this person may think with: the second is narrower than the
// deployment's when the organisation limited them to some models.
func (s *Server) enginesJSON(w http.ResponseWriter, r *http.Request) {
	offered := s.offeredTo(r)
	out := []engineJSON{}
	for _, entry := range s.engines() {
		models := offered
		if models == nil {
			models = []string{}
		}
		out = append(out, engineJSON{Name: entry.Name, Title: entry.Title, What: entry.What, Ready: entry.Ready, Why: entry.Why, Models: models})
	}
	answerJSON(w, http.StatusOK, out)
}

// startInstanceJSON starts an agent for the person, under the same rules as the page: what this deployment can run, what
// they may use, and where a repository may come from.
func (s *Server) startInstanceJSON(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Harness string `json:"harness"`
		Model   string `json:"model"`
		Repo    string `json:"repo"`
		Setup   string `json:"setup"`
		Title   string `json:"title"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&asked) != nil {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": "send JSON: harness, and optionally model, repo, setup and title"})
		return
	}
	started, _, why := s.beginInstance(r, instance.Instance{
		Harness: strings.TrimSpace(asked.Harness), Model: strings.TrimSpace(asked.Model), Repo: strings.TrimSpace(asked.Repo),
		Setup: strings.TrimSpace(asked.Setup), Title: strings.TrimSpace(asked.Title), Who: personOf(r),
	})
	if why != "" {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": why})
		return
	}
	answerJSON(w, http.StatusCreated, map[string]any{"instance": started, "thread": s.instances.Latest(started.ID)})
}

// stepJSON is one thing an agent did, in the words the pages use.
type stepJSON struct {
	What   string `json:"what"`
	Detail string `json:"detail,omitempty"`
}

func stepsJSON(steps []step) []stepJSON {
	out := make([]stepJSON, 0, len(steps))
	for _, each := range steps {
		out = append(out, stepJSON{What: each.What, Detail: each.Detail})
	}
	return out
}

// turnJSON is one turn as a program reads it: how it stands, what the agent did, what it said, and which files it changed.
type turnJSON struct {
	Job     jobs.Record `json:"job"`
	Steps   []stepJSON  `json:"steps"`
	Reply   string      `json:"reply"`
	Changed []string    `json:"changed"`
	Stopped string      `json:"stopped,omitempty"`
}

// ownJob is the job the path names, when the person asking may read it.
func (s *Server) ownJob(r *http.Request) (jobs.Record, bool) {
	record, found := s.index.Get(r.PathValue("id"))
	if !found || !s.mayUse(r, record.Asked) {
		return jobs.Record{}, false
	}
	return record, true
}

func (s *Server) turnOf(record jobs.Record) turnJSON {
	dir := record.Dir(s.index.Root())
	shown := turnJSON{Job: record, Steps: stepsJSON(stepsOf(dir)), Reply: said(dir), Changed: changed(dir)}
	if shown.Changed == nil {
		shown.Changed = []string{}
	}
	if record.State == jobs.Failed || record.State == jobs.Interrupted {
		shown.Stopped = stopped(dir)
	}
	return shown
}

func (s *Server) jobJSON(w http.ResponseWriter, r *http.Request) {
	record, found := s.ownJob(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	answerJSON(w, http.StatusOK, s.turnOf(record))
}

// followJSON streams one turn as it goes: each step the agent takes as `step`, what it has said so far as `reply` whenever
// that changes, and `ended` with the whole turn when it is over. A turn that is already over is told at once, so following
// one is always safe.
func (s *Server) followJSON(w http.ResponseWriter, r *http.Request) {
	record, found := s.ownJob(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	if _, streams := w.(http.Flusher); !streams {
		http.Error(w, "this connection cannot stream", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	stream := &feed{to: w, control: http.NewResponseController(w)}
	stream.flush()

	sent, last := 0, ""
	ticker := time.NewTicker(followEvery)
	defer ticker.Stop()
	for {
		record = s.latest(record)
		now := s.turnOf(record)
		for ; sent < len(now.Steps); sent++ {
			body, _ := json.Marshal(now.Steps[sent])
			stream.send("event: step\ndata: %s\n\n", body)
		}
		if now.Reply != last && !record.State.Done() {
			last = now.Reply
			body, _ := json.Marshal(map[string]string{"text": now.Reply})
			stream.send("event: reply\ndata: %s\n\n", body)
		}
		if record.State.Done() {
			body, _ := json.Marshal(now)
			stream.send("event: ended\ndata: %s\n\n", body)
			stream.flush()
			return
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

// followEvery is how often a followed turn is looked at.
var followEvery = 300 * time.Millisecond
