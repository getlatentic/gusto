package helper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Turn is one of the person's jobs as the server's event stream tells it, the latest word on it.
type Turn struct {
	Job      string `json:"job"`
	Instance string `json:"instance,omitempty"`
	Thread   string `json:"thread,omitempty"`
	Brief    string `json:"brief"`
	State    string `json:"state"`
	At       int64  `json:"at"`
}

// Event is a turn's change as a shell hears it: the kind the server named, such as turn.finished, and the turn.
type Event struct {
	Kind string `json:"kind"`
	Turn Turn   `json:"turn"`
}

// Hub holds the latest word on each turn and tells everybody listening when one changes. It is how one connection to the
// server serves every shell on the computer.
type Hub struct {
	mu        sync.Mutex
	turns     map[string]Turn
	order     []string
	listeners map[chan Event]struct{}
	// last is the time of the newest event heard, which a reconnect asks the server to catch up from.
	last int64
}

// NewHub is a hub that has heard nothing.
func NewHub() *Hub {
	return &Hub{turns: map[string]Turn{}, listeners: map[chan Event]struct{}{}}
}

// Tell records an event and passes it on. A listener that is not keeping up loses the event and not the others' time, and
// finds the turn's latest word the next time it asks for the turns.
func (h *Hub) Tell(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, known := h.turns[event.Turn.Job]; !known {
		h.order = append(h.order, event.Turn.Job)
	}
	h.turns[event.Turn.Job] = event.Turn
	h.last = max(h.last, event.Turn.At)
	for listener := range h.listeners {
		select {
		case listener <- event:
		default:
		}
	}
}

// Turns is the latest word on every turn heard, the newest first.
func (h *Hub) Turns() []Turn {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Turn, 0, len(h.order))
	for i := len(h.order) - 1; i >= 0; i-- {
		out = append(out, h.turns[h.order[i]])
	}
	return out
}

// Since is the time to ask the server to catch up from after a connection was lost, with a minute's margin for the clock.
func (h *Hub) Since() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.last == 0 {
		return 0
	}
	return max(h.last-60, 1)
}

// Listen is the events from now on, and a way to stop listening.
func (h *Hub) Listen() (<-chan Event, func()) {
	listener := make(chan Event, 64)
	h.mu.Lock()
	h.listeners[listener] = struct{}{}
	h.mu.Unlock()
	return listener, func() {
		h.mu.Lock()
		delete(h.listeners, listener)
		h.mu.Unlock()
	}
}

// ErrSignedOut is the server saying the token is no longer one it takes.
var ErrSignedOut = errors.New("the server no longer accepts this computer's token")

// Follow holds one connection to the server's event stream and tells the hub what comes over it, until the connection ends,
// which it says with an error. A token the server refuses is ErrSignedOut.
func (c Client) Follow(ctx context.Context, hub *Hub, connected func(bool)) error {
	address := strings.TrimRight(c.Server, "/") + "/api/events"
	if since := hub.Since(); since > 0 {
		address += fmt.Sprintf("?since=%d", since)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Accept", "text/event-stream")
	// A stream has no overall deadline, so the client given has none either.
	response, err := (&http.Client{Transport: c.transport()}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return ErrSignedOut
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("the event stream answered %d", response.StatusCode)
	}
	connected(true)
	defer connected(false)
	return readEvents(response.Body, hub)
}

func (c Client) transport() http.RoundTripper {
	if c.HTTP != nil && c.HTTP.Transport != nil {
		return c.HTTP.Transport
	}
	return http.DefaultTransport
}

// readEvents turns server-sent frames into the hub's events, until the stream ends.
func readEvents(body io.Reader, hub *Hub) error {
	lines := bufio.NewScanner(body)
	lines.Buffer(make([]byte, 64<<10), 1<<20)
	var kind, data string
	for lines.Scan() {
		line := lines.Text()
		switch {
		case line == "":
			if strings.HasPrefix(kind, "turn.") {
				var turn Turn
				if json.Unmarshal([]byte(data), &turn) == nil && turn.Job != "" {
					hub.Tell(Event{Kind: kind, Turn: turn})
				}
			}
			kind, data = "", ""
		case strings.HasPrefix(line, "event:"):
			kind = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	if err := lines.Err(); err != nil {
		return err
	}
	return io.EOF
}

// Backoff is how long to wait before the next try to reach the server, doubling from a second to half a minute.
func Backoff(attempt int) time.Duration {
	wait := time.Second << min(attempt, 5)
	return min(wait, 30*time.Second)
}
