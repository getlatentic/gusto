package web

import (
	"net/http"
	"strings"
)

// liveStep is one thing a turn has done so far.
type liveStep struct {
	What   string `json:"what"`
	Detail string `json:"detail,omitempty"`
}

// liveTurn is what a turn that is running has done so far, for the page to show while it waits: its last few steps, and the tail of what
// it has said.
type liveTurn struct {
	Steps []liveStep `json:"steps"`
	Count int        `json:"count"`
	Said  string     `json:"said,omitempty"`
}

const (
	liveSteps = 5
	liveSaid  = 400
)

// turnLive answers what the thread's running turn has done so far.
func (s *Server) turnLive(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found || len(thread.Turns) == 0 {
		http.NotFound(w, r)
		return
	}
	record, found := s.index.Get(thread.Turns[len(thread.Turns)-1])
	if !found {
		http.NotFound(w, r)
		return
	}
	dir := record.Dir(s.index.Root())
	steps := stepsOf(dir)
	live := liveTurn{Count: len(steps), Steps: []liveStep{}}
	for _, each := range steps[max(0, len(steps)-liveSteps):] {
		live.Steps = append(live.Steps, liveStep{What: each.What, Detail: each.Detail})
	}
	said := strings.TrimSpace(strings.ToValidUTF8(said(dir), ""))
	if runes := []rune(said); len(runes) > liveSaid {
		said = "…" + string(runes[len(runes)-liveSaid:])
	}
	live.Said = said
	answerJSON(w, http.StatusOK, live)
}
