package roomlive

import (
	"context"
	"errors"
	"net/http"

	"github.com/getlatentic/gusto/internal/roomlog"
)

// What the live layer asks of the rest of the server. The store gives the log, the pages draw an event, and the routes' own
// rules decide who may do what, so a command on the socket can do no more than the same person could on the page.

// Log is a room's event log.
type Log interface {
	// RoomEvents is the events after a sequence number, at most limit, and where the stream stands.
	RoomEvents(room string, after uint64, limit int) (roomlog.Slice, error)
	// RoomHead is where the stream stands.
	RoomHead(room string) (roomlog.Position, error)
}

// Drawn is an event as viewers are shown it: the HTML of a turn or note, drawn once and sent to everybody.
type Drawn struct {
	Type   string
	Thread string
	ID     string
	HTML   string
}

// Renderer draws an event from the state as it is now.
type Renderer interface {
	Draw(ctx context.Context, room string, event roomlog.Event) (Drawn, error)
}

// Viewer is a person with a socket open. Who is the name the server knows them by, Name is what a room calls them, Info is what
// the others see of them in the room's presence, and Session is whatever the Backend needs to ask again whether they are still
// allowed.
type Viewer struct {
	Who     string
	Name    string
	Info    []byte
	Session any
}

// Backend is the server's side of the live layer.
type Backend interface {
	Log
	Renderer
	// Authenticate says who is asking from the request that opens the socket, or why they may not.
	Authenticate(r *http.Request) (Viewer, error)
	// Alive says whether the person's session is still good.
	Alive(viewer Viewer) bool
	// Authorize says whether the person may watch and speak in the room now.
	Authorize(viewer Viewer, room string) error
	// Do carries out a command as the person, after checking again that they may.
	Do(ctx context.Context, viewer Viewer, command Command) (Result, error)
}

// The reasons a person is refused. A room that is not there and one the person is not in are the same answer, so a guess cannot
// tell the two apart.
var (
	ErrUnauthenticated = errors.New("sign in first")
	ErrNotFound        = errors.New("no such room")
)
