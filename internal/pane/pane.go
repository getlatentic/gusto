// Package pane is a terminal a job runs on, held by the server so that people
// can watch it and type into it while it runs.
//
// The box draws the screen; a pane only carries it. Everything drawn so far is
// kept, so somebody who opens the pane late is shown the screen as it stands
// rather than whatever the next byte happens to be, and two people watching
// see the same thing. Whatever any of them types goes to the one terminal.
package pane

import (
	"io"
	"sync"
)

// kept is how much of what a pane drew it holds for a watcher who arrives
// late. A screen is redrawn often enough that the end of a long session
// rebuilds it; the whole of it is in the job's pane.cast.
const kept = 4 << 20

// Pane is one running terminal: what it drew, who is watching, and the way in.
type Pane struct {
	guard    sync.Mutex
	drawn    []byte
	watchers map[chan []byte]struct{}
	ended    bool
	typed    *io.PipeReader
	typing   *io.PipeWriter
}

func open() *Pane {
	typed, typing := io.Pipe()
	return &Pane{watchers: map[chan []byte]struct{}{}, typed: typed, typing: typing}
}

// Write is the box drawing. Each watcher is sent the bytes; one too slow to
// keep up is let go, and it reconnects to the screen as it stands.
func (p *Pane) Write(drawn []byte) (int, error) {
	p.guard.Lock()
	defer p.guard.Unlock()
	p.drawn = append(p.drawn, drawn...)
	if over := len(p.drawn) - kept; over > 0 {
		p.drawn = append([]byte(nil), p.drawn[over:]...)
	}
	piece := append([]byte(nil), drawn...)
	for watcher := range p.watchers {
		select {
		case watcher <- piece:
		default:
			delete(p.watchers, watcher)
			close(watcher)
		}
	}
	return len(drawn), nil
}

// Read is what the box is typed: whatever anybody watching sends.
func (p *Pane) Read(into []byte) (int, error) { return p.typed.Read(into) }

// Type sends keys to the terminal. A pane that has ended takes none.
func (p *Pane) Type(keys []byte) error {
	p.guard.Lock()
	ended := p.ended
	p.guard.Unlock()
	if ended {
		return io.ErrClosedPipe
	}
	_, err := p.typing.Write(keys)
	return err
}

// Watch is the screen so far and what is drawn from now on. The channel
// closes when the pane ends or lets the watcher go; stop lets it go early.
func (p *Pane) Watch() (screen []byte, drawn <-chan []byte, stop func()) {
	p.guard.Lock()
	defer p.guard.Unlock()
	updates := make(chan []byte, 256)
	screen = append([]byte(nil), p.drawn...)
	if p.ended {
		close(updates)
		return screen, updates, func() {}
	}
	p.watchers[updates] = struct{}{}
	return screen, updates, func() {
		p.guard.Lock()
		defer p.guard.Unlock()
		if _, held := p.watchers[updates]; held {
			delete(p.watchers, updates)
			close(updates)
		}
	}
}

func (p *Pane) end() {
	p.guard.Lock()
	defer p.guard.Unlock()
	p.ended = true
	for watcher := range p.watchers {
		close(watcher)
	}
	p.watchers = map[chan []byte]struct{}{}
	p.typing.Close()
}

// Hub is every pane running on this server, by the job it belongs to.
type Hub struct {
	guard sync.Mutex
	panes map[string]*Pane
}

func NewHub() *Hub { return &Hub{panes: map[string]*Pane{}} }

// Open starts a pane for a job, and the returned func ends it. An ended pane
// is gone from the hub: what it drew is the job's evidence from then on.
func (h *Hub) Open(job string) (*Pane, func()) {
	opened := open()
	h.guard.Lock()
	h.panes[job] = opened
	h.guard.Unlock()
	return opened, func() {
		opened.end()
		h.guard.Lock()
		defer h.guard.Unlock()
		if h.panes[job] == opened {
			delete(h.panes, job)
		}
	}
}

// Get is a job's pane while it runs.
func (h *Hub) Get(job string) (*Pane, bool) {
	h.guard.Lock()
	defer h.guard.Unlock()
	held, found := h.panes[job]
	return held, found
}
