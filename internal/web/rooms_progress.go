package web

import (
	"net/http"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/tasks"
)

// Several agents can be working in a room at once, and each turn shows what its agent is doing now. A room's page asks for them
// all in one request, and what is read from a running turn's files is kept for a moment, so that however many people have the room
// open the files are read once for them.

// turnProgress is what a running turn is doing: the latest step it took, and how many it has taken.
type turnProgress struct {
	What   string `json:"what,omitempty"`
	Detail string `json:"detail,omitempty"`
	Steps  int    `json:"steps"`
}

// progressLife is how long a turn's progress is kept before its files are read again, a little under how often a page asks.
const progressLife = 1500 * time.Millisecond

// mostProgress bounds the progress kept; at the bound it is all dropped and read again as it is asked for.
const mostProgress = 4096

type progressNote struct {
	at       time.Time
	progress turnProgress
}

// progressKept is the progress of running turns by turn id.
type progressKept struct {
	guard sync.Mutex
	held  map[string]progressNote
}

func (p *progressKept) get(id string, now time.Time) (turnProgress, bool) {
	p.guard.Lock()
	defer p.guard.Unlock()
	note, found := p.held[id]
	return note.progress, found && now.Sub(note.at) < progressLife
}

func (p *progressKept) put(id string, progress turnProgress, now time.Time) {
	p.guard.Lock()
	defer p.guard.Unlock()
	if p.held == nil || len(p.held) >= mostProgress {
		p.held = map[string]progressNote{}
	}
	p.held[id] = progressNote{at: now, progress: progress}
}

// progressOf is what a running turn has done, from its record of commands.
func (s *Server) progressOf(record tasks.Record) turnProgress {
	now := time.Now()
	if kept, fresh := s.progress.get(record.ID, now); fresh {
		return kept
	}
	ran := ranBy(record.Dir(s.index.Root()))
	progress := turnProgress{Steps: len(ran)}
	if len(ran) > 0 {
		latest := describe(ran[len(ran)-1])
		progress.What, progress.Detail = latest.What, latest.Detail
	}
	s.progress.put(record.ID, progress, now)
	return progress
}

// roomProgress answers what each turn that is running in a conversation is doing, by turn id. A turn that is waiting to start has
// no progress, and says what it waits for in the turn itself.
func (s *Server) roomProgress(w http.ResponseWriter, r *http.Request) {
	held, found := s.sharedOr404(w, r)
	if !found {
		return
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found {
		http.NotFound(w, r)
		return
	}
	doing := map[string]turnProgress{}
	for _, id := range thread.Turns {
		if record, found := s.index.Get(id); found && record.State == tasks.Running {
			doing[id] = s.progressOf(record)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	answerJSON(w, http.StatusOK, doing)
}
