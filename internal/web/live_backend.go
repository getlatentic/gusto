package web

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/roomlog"
)

// liveBackend is the page's side of the live layer. A person on a socket is held to what they could do on the page: who they
// are comes from the same session cookie, and whether they may see or speak in a room from the same rules, asked of a request
// made to look like the one they would have sent.
type liveBackend struct{ s *Server }

var _ roomlive.Backend = liveBackend{}

// liveSession is what is kept of the request that opened a socket, to ask the rules again later: the session cookie, and
// whether the connection was secure, since the cookie's name depends on it.
type liveSession struct {
	cookie string
	proto  string
	secure bool
}

// request makes a request that carries the session, for the room named, to ask the rules of.
func (l liveSession) request(room string) *http.Request {
	header := http.Header{}
	if l.cookie != "" {
		header.Set("Cookie", l.cookie)
	}
	if l.proto != "" {
		header.Set("X-Forwarded-Proto", l.proto)
	}
	r := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/"}, Header: header}
	if l.secure {
		r.TLS = &tls.ConnectionState{}
	}
	r.SetPathValue("id", room)
	return r
}

func (b liveBackend) Authenticate(r *http.Request) (roomlive.Viewer, error) {
	if !b.s.keeper.knows(r) {
		return roomlive.Viewer{}, roomlive.ErrUnauthenticated
	}
	who := b.s.askerName(r)
	info, _ := json.Marshal(map[string]string{"name": b.s.personName(who), "short": b.s.initials(who)})
	session := liveSession{cookie: r.Header.Get("Cookie"), proto: r.Header.Get("X-Forwarded-Proto"), secure: r.TLS != nil}
	return roomlive.Viewer{Who: who, Name: b.s.personName(who), Info: info, Session: session}, nil
}

func (b liveBackend) Alive(viewer roomlive.Viewer) bool {
	return b.s.keeper.knows(sessionOf(viewer).request(""))
}

// Authorize asks the rule every route that opens a room asks: the person is signed in, and is in the room or runs the server.
func (b liveBackend) Authorize(viewer roomlive.Viewer, room string) error {
	r := sessionOf(viewer).request(room)
	if !b.s.keeper.knows(r) {
		return roomlive.ErrUnauthenticated
	}
	if held, found := b.s.shared(r); !found || !held.Room {
		return roomlive.ErrNotFound
	}
	return nil
}

func sessionOf(viewer roomlive.Viewer) liveSession {
	session, _ := viewer.Session.(liveSession)
	return session
}

func (b liveBackend) RoomEvents(room string, after uint64, limit int) (roomlog.Slice, error) {
	return b.s.instances.RoomEvents(room, after, limit)
}

func (b liveBackend) RoomHead(room string) (roomlog.Position, error) {
	return b.s.instances.RoomHead(room)
}

// Draw draws what an event changed: the turn or note it names, from the state as it is, with the same templates as the page.
func (b liveBackend) Draw(ctx context.Context, room string, event roomlog.Event) (roomlive.Drawn, error) {
	return b.s.drawEvent(ctx, room, event)
}

// Do carries out a command as the person, by the same functions the page's routes call, after asking the rules again.
func (b liveBackend) Do(ctx context.Context, viewer roomlive.Viewer, command roomlive.Command) (roomlive.Result, error) {
	if err := b.Authorize(viewer, command.Room); err != nil {
		return roomlive.Result{}, err
	}
	r := sessionOf(viewer).request(command.Room).WithContext(ctx)
	held, found := b.s.shared(r)
	if !found || !held.Room {
		return roomlive.Result{}, roomlive.ErrNotFound
	}
	switch command.Name {
	case roomlive.CmdSay:
		return b.s.liveSay(r, held, command), nil
	case roomlive.CmdStop:
		return b.s.liveStop(r, held, command), nil
	case roomlive.CmdSeen:
		return b.s.liveSeen(r, held, command), nil
	}
	return roomlive.Result{Error: "that command is not known"}, nil
}
