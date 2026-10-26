package web

import (
	"context"
	"net/url"
	"sync"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/roomlive"
)

// A room's page follows the room over one WebSocket. The store's event log says what happened; this file connects it to the
// live layer: it starts it, gives it the page's routes and rules, and turns a job's changes into events.

// livePath is where a browser opens the socket.
const livePath = "/live"

// liveStream is where a room's event stream stood when its page was drawn, which is where the page's socket follows from.
type liveStream struct {
	Path  string
	Seq   uint64
	Epoch string
}

// streamOf is where a room's stream stands now, or nothing when the room is not live.
func (s *Server) streamOf(held instance.Instance) *liveStream {
	if s.sockets == nil || !held.Room {
		return nil
	}
	head, err := s.instances.RoomHead(held.ID)
	if err != nil {
		return nil
	}
	return &liveStream{Path: livePath + "?room=" + url.QueryEscape(held.ID), Seq: head.Seq, Epoch: head.Epoch}
}

// UseLive starts the live layer: the sockets of the people looking at rooms. Without it a room's page asks whether anything
// changed every few seconds, which still works.
func (s *Server) UseLive(opts roomlive.Options) error {
	live, err := roomlive.New(liveBackend{s}, opts)
	if err != nil {
		return err
	}
	s.sockets = live
	s.instances.OnRoomEvent(live.Wake)
	s.announce = newAnnouncer(s.instances)
	s.index.OnChange(s.announce.note)
	// A turn that was running when the server before this one stopped has ended as interrupted, and the people who were watching
	// it have no event for that.
	for _, record := range s.index.Recovered() {
		s.announce.note(record)
	}
	return nil
}

// CloseLive ends every socket and the goroutine that records job changes, and waits for them or for ctx.
func (s *Server) CloseLive(ctx context.Context) error {
	if s.sockets == nil {
		return nil
	}
	s.announce.stop()
	return s.sockets.Shutdown(ctx)
}

// liveMetrics reports what the live layer is doing, when there is one.
func (s *Server) liveMetrics(out *exposition) {
	if s.sockets == nil {
		return
	}
	stats := s.sockets.Stats()
	out.gauge("gusto_live_connections", "WebSockets open: the room pages that are being kept up to date.", float64(stats.Connections))
	out.gauge("gusto_live_rooms_followed", "Rooms with somebody watching them live.", float64(stats.Rooms))
	out.counter("gusto_live_events_published_total", "Room events handed to the people watching, since this server started.", float64(stats.Published))
	out.counter("gusto_live_events_drawn_total", "Room events drawn as HTML, once for everybody watching.", float64(stats.Drawn))
	out.counter("gusto_live_events_replayed_from_cache_total", "Events a reconnecting page was sent from what was already drawn.", float64(stats.Replayed))
	out.counter("gusto_live_commands_total", "Commands people sent over their sockets: saying, stopping and having read.", float64(stats.Commands))
	out.counter("gusto_live_commands_repeated_total", "Of them, commands sent twice, which were done once.", float64(stats.Repeated))
}

// announcer turns the changes of jobs into events in their rooms' logs. A job's change is noted under the index's lock, so it
// is only queued here, and a goroutine writes the events. A turn that changes twice before its event is written is one event,
// since an event says that something changed and the page is drawn from the state as it is when it is sent.
type announcer struct {
	instances *instance.Store
	wake      chan struct{}
	done      chan struct{}
	ended     chan struct{}

	mu      sync.Mutex
	stopped bool
	pending []pendingTurn
	queued  map[pendingTurn]struct{}
}

type pendingTurn struct{ room, thread, turn string }

func newAnnouncer(instances *instance.Store) *announcer {
	a := &announcer{
		instances: instances, wake: make(chan struct{}, 1), done: make(chan struct{}), ended: make(chan struct{}),
		queued: map[pendingTurn]struct{}{},
	}
	go a.run()
	return a
}

// note queues the change of a job that is a turn in a conversation. A job that is only queued is announced by the store when it
// is added to the conversation.
func (a *announcer) note(record jobs.Record) {
	if record.Instance == "" || record.Thread == "" || record.State == jobs.Queued {
		return
	}
	turn := pendingTurn{record.Instance, record.Thread, record.ID}
	a.mu.Lock()
	if _, waiting := a.queued[turn]; !waiting && !a.stopped {
		a.queued[turn] = struct{}{}
		a.pending = append(a.pending, turn)
	}
	a.mu.Unlock()
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *announcer) run() {
	defer close(a.ended)
	for {
		select {
		case <-a.done:
			a.flush()
			return
		case <-a.wake:
			a.flush()
		}
	}
}

func (a *announcer) flush() {
	for {
		a.mu.Lock()
		if len(a.pending) == 0 {
			a.mu.Unlock()
			return
		}
		turn := a.pending[0]
		a.pending = a.pending[1:]
		delete(a.queued, turn)
		a.mu.Unlock()
		a.instances.Record(turn.room, turn.thread, instance.KindTurn, turn.turn)
	}
}

func (a *announcer) stop() {
	a.mu.Lock()
	a.stopped = true
	a.mu.Unlock()
	select {
	case <-a.done:
	default:
		close(a.done)
	}
	<-a.ended
}
