package roomlive

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roomlog"
)

// fakeBackend is a Backend over a real room log on disk, with people as the tests say who is in which room.
type fakeBackend struct {
	log *roomlog.Set
	dir string

	mu      sync.Mutex
	members map[string]map[string]bool
	revoked map[string]bool
	did     []Command

	// gate, when set, holds every draw until it is closed.
	gate atomic.Pointer[chan struct{}]
	// bulk is the size of padding added to each drawn page, for tests that need large events.
	bulk   int
	draws  atomic.Int64
	server *Server
}

func newFakeBackend(dir string, window int) *fakeBackend {
	b := &fakeBackend{dir: dir, members: map[string]map[string]bool{}, revoked: map[string]bool{}}
	b.log = roomlog.NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, roomlog.Options{Window: window})
	return b
}

func (b *fakeBackend) addRoom(room string, who ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.members[room] = map[string]bool{}
	for _, each := range who {
		b.members[room][each] = true
	}
}

func (b *fakeBackend) removeMember(room, who string) {
	b.mu.Lock()
	delete(b.members[room], who)
	b.mu.Unlock()
}

func (b *fakeBackend) revoke(who string) {
	b.mu.Lock()
	b.revoked[who] = true
	b.mu.Unlock()
}

// emit is a change to a room: it is logged, and the live layer is woken, as the store does.
func (b *fakeBackend) emit(room, kind, ref string) roomlog.Event {
	event, err := b.log.Append(room, roomlog.Event{Kind: kind, Thread: "1", Ref: ref})
	if err != nil {
		panic(err)
	}
	if b.server != nil {
		b.server.Wake(room)
	}
	return event
}

func (b *fakeBackend) RoomEvents(room string, after uint64, limit int) (roomlog.Slice, error) {
	if !b.known(room) {
		return roomlog.Slice{}, ErrNotFound
	}
	return b.log.After(room, after, limit)
}

func (b *fakeBackend) RoomHead(room string) (roomlog.Position, error) {
	if !b.known(room) {
		return roomlog.Position{}, ErrNotFound
	}
	return b.log.Head(room)
}

func (b *fakeBackend) known(room string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, found := b.members[room]
	return found
}

func (b *fakeBackend) Draw(ctx context.Context, room string, event roomlog.Event) (Drawn, error) {
	b.draws.Add(1)
	if gate := b.gate.Load(); gate != nil {
		select {
		case <-*gate:
		case <-ctx.Done():
			return Drawn{}, ctx.Err()
		}
	}
	html := fmt.Sprintf(`<article id="%s">%s #%d</article>`, event.Ref, event.Kind, event.Seq)
	if b.bulk > 0 {
		html += strings.Repeat("x", b.bulk)
	}
	return Drawn{Type: event.Kind, Thread: event.Thread, ID: event.Ref, HTML: html}, nil
}

func (b *fakeBackend) Authenticate(r *http.Request) (Viewer, error) {
	who := r.Header.Get("X-Who")
	if who == "" {
		return Viewer{}, ErrUnauthenticated
	}
	return Viewer{Who: who, Name: who, Info: []byte(fmt.Sprintf(`{"name":%q}`, who))}, nil
}

func (b *fakeBackend) Alive(viewer Viewer) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.revoked[viewer.Who]
}

func (b *fakeBackend) Authorize(viewer Viewer, room string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.revoked[viewer.Who] || !(b.members[room][viewer.Who] || b.members[room]["*"]) {
		return ErrNotFound
	}
	return nil
}

// Do says a message makes a turn, the way the page does: it is carried out once per command id by the live layer's dedupe,
// and here it also checks the person again, as the real backend does.
func (b *fakeBackend) Do(_ context.Context, viewer Viewer, command Command) (Result, error) {
	if err := b.Authorize(viewer, command.Room); err != nil {
		return Result{}, err
	}
	b.mu.Lock()
	b.did = append(b.did, command)
	count := len(b.did)
	b.mu.Unlock()
	ref := fmt.Sprintf("t%d", count)
	b.emit(command.Room, "turn", ref)
	return Result{OK: true, Ref: ref}, nil
}

func (b *fakeBackend) done() []Command {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Command(nil), b.did...)
}

// rig is a live layer on a test HTTP server.
type rig struct {
	t       *testing.T
	backend *fakeBackend
	server  *Server
	http    *httptest.Server
}

func newRig(t *testing.T, window int, opts Options) *rig {
	t.Helper()
	return newRigOn(t, newFakeBackend(t.TempDir(), window), opts)
}

func newRigOn(t *testing.T, backend *fakeBackend, opts Options) *rig {
	t.Helper()
	server, err := New(backend, opts)
	if err != nil {
		t.Fatal(err)
	}
	backend.server = server
	mux := http.NewServeMux()
	mux.Handle("/live", server.Handler())
	r := &rig{t: t, backend: backend, server: server, http: httptest.NewServer(mux)}
	t.Cleanup(r.stop)
	return r
}

func (r *rig) stop() {
	r.http.CloseClientConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.server.Shutdown(ctx)
	r.http.Close()
}

// dial connects a person, who watches the room's stream and live channel unless they ask otherwise.
func (r *rig) dial(who, room string) *client {
	r.t.Helper()
	c, status, err := connectClient(r.t, r.http.URL, "/live?room="+room, who, false)
	if err != nil {
		r.t.Fatalf("%s could not connect to %s: %v (status %d)", who, room, err, status)
	}
	r.t.Cleanup(c.close)
	return c
}

// join subscribes a connected client to the room's stream, from the head, and its live channel.
func (r *rig) join(c *client, room string) subscribed {
	r.t.Helper()
	got := c.subscribe(StreamChannel(room), nil)
	if got.Err != nil {
		r.t.Fatalf("subscribe: %s", got.Err.Message)
	}
	if live := c.subscribe(LiveChannel(room), nil); live.Err != nil {
		r.t.Fatalf("subscribe live: %s", live.Err.Message)
	}
	return got
}

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func eventually(t testing.TB, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
