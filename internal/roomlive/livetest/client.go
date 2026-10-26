package livetest

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// PushBuffer is how many pushes a client holds for its reader before a full buffer is an error. A load run with many clients
// lowers it, and drains each.
var PushBuffer = 8192

// Client is a socket that has connected to the Centrifuge protocol and is read in the background.
type Client struct {
	T      testing.TB
	ws     *Conn
	nextID atomic.Uint64
	mu     sync.Mutex
	wait   map[uint64]chan Answer
	// Pushes are the publications and other pushes, in the order they arrived.
	Pushes chan Push
	// Closed is closed when the socket ends, and CloseCode says why: the code of the closing frame, or -1 when the connection was
	// cut off.
	Closed    chan struct{}
	CloseCode atomic.Int64
	// Hello is what the server said when the client connected: its ping interval among it.
	Hello json.RawMessage
	// stalled holds the reader still, to make a client that does not take what it is sent.
	stalled atomic.Bool
}

// Answer is the server's reply to a command.
type Answer struct {
	Error *Error
	Raw   map[string]json.RawMessage
}

// Error is an error reply.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Push is something the server sent that was not a reply.
type Push struct {
	Channel     string
	Offset      uint64
	Data        json.RawMessage
	Join        *Info
	Leave       *Info
	Unsubscribe *struct{ Code int }
}

// Info is who joined or left a channel.
type Info struct {
	User     string          `json:"user"`
	Client   string          `json:"client"`
	ConnInfo json.RawMessage `json:"conn_info"`
}

// Position is a place in a stream.
type Position struct {
	Offset uint64
	Epoch  string
}

// Subscribed is what a subscribe reply says.
type Subscribed struct {
	Err       *Error
	Recovered bool
	Offset    uint64
	Epoch     string
	Pubs      []Push
}

// Connect opens a socket as the person whose cookie header is given, or as nobody, and connects to the protocol.
func Connect(t testing.TB, base, path string, header http.Header, stalled bool) (*Client, int, error) {
	t.Helper()
	ws, status, err := DialWS(base, path, header)
	if err != nil {
		return nil, status, err
	}
	c := &Client{T: t, ws: ws, wait: map[uint64]chan Answer{}, Pushes: make(chan Push, PushBuffer), Closed: make(chan struct{})}
	c.stalled.Store(stalled)
	go c.read()
	r := c.Call(map[string]any{"connect": map[string]any{"name": "test"}})
	if r.Error != nil {
		return nil, status, fmt.Errorf("connect: %s", r.Error.Message)
	}
	c.Hello = r.Raw["connect"]
	return c, status, nil
}

// WriteRaw sends a frame as it is given, for a test of how the server takes one it should refuse.
func (c *Client) WriteRaw(opcode byte, payload []byte) error { return c.ws.WriteFrame(opcode, payload) }

// Stall holds the reader still, or lets it go on.
func (c *Client) Stall(on bool) { c.stalled.Store(on) }

func (c *Client) read() {
	defer close(c.Closed)
	for {
		for c.stalled.Load() {
			time.Sleep(5 * time.Millisecond)
		}
		opcode, payload, err := c.ws.ReadFrame()
		if err != nil {
			c.CloseCode.CompareAndSwap(0, -1)
			return
		}
		switch opcode {
		case 0x8:
			code := int64(-2)
			if len(payload) >= 2 {
				code = int64(binary.BigEndian.Uint16(payload))
			}
			c.CloseCode.Store(code)
			_ = c.ws.WriteFrame(0x8, payload)
			return
		case 0x9:
			_ = c.ws.WriteFrame(0xA, payload)
		case 0x1:
			c.dispatch(payload)
		}
	}
}

// dispatch handles the lines of one frame: a ping, a reply to a command, or a push.
func (c *Client) dispatch(payload []byte) {
	for _, line := range bytes.Split(payload, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil {
			c.T.Errorf("not JSON: %.200s", line)
			continue
		}
		switch {
		case len(fields) == 0:
			_ = c.ws.WriteFrame(0x1, []byte("{}"))
		case fields["push"] != nil:
			c.handlePush(fields["push"])
		default:
			c.handleReply(fields)
		}
	}
}

func (c *Client) handleReply(fields map[string]json.RawMessage) {
	var id uint64
	_ = json.Unmarshal(fields["id"], &id)
	answer := Answer{Raw: fields}
	if fields["error"] != nil {
		answer.Error = &Error{}
		_ = json.Unmarshal(fields["error"], answer.Error)
	}
	c.mu.Lock()
	waiting := c.wait[id]
	delete(c.wait, id)
	c.mu.Unlock()
	if waiting != nil {
		waiting <- answer
	}
}

func (c *Client) handlePush(raw json.RawMessage) {
	var body struct {
		Channel string `json:"channel"`
		Pub     *struct {
			Offset uint64          `json:"offset"`
			Data   json.RawMessage `json:"data"`
		} `json:"pub"`
		Join  *struct{ Info Info } `json:"join"`
		Leave *struct{ Info Info } `json:"leave"`
		Unsub *struct{ Code int }  `json:"unsubscribe"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		c.T.Errorf("bad push: %.200s", raw)
		return
	}
	p := Push{Channel: body.Channel}
	switch {
	case body.Pub != nil:
		p.Offset, p.Data = body.Pub.Offset, body.Pub.Data
	case body.Join != nil:
		p.Join = &body.Join.Info
	case body.Leave != nil:
		p.Leave = &body.Leave.Info
	case body.Unsub != nil:
		p.Unsubscribe = body.Unsub
	default:
		return
	}
	select {
	case c.Pushes <- p:
	default:
		c.T.Errorf("a client's push queue is full")
	}
}

// Call sends a command and waits for its reply.
func (c *Client) Call(command map[string]any) Answer {
	id := c.nextID.Add(1)
	command["id"] = id
	data, _ := json.Marshal(command)
	waiting := make(chan Answer, 1)
	c.mu.Lock()
	c.wait[id] = waiting
	c.mu.Unlock()
	if err := c.ws.WriteFrame(0x1, data); err != nil {
		return Answer{Error: &Error{Message: err.Error()}}
	}
	select {
	case a := <-waiting:
		return a
	case <-c.Closed:
		return Answer{Error: &Error{Message: "closed"}}
	case <-time.After(10 * time.Second):
		return Answer{Error: &Error{Message: "no reply"}}
	}
}

// Subscribe joins a channel, recovering from a position when one is given.
func (c *Client) Subscribe(channel string, since *Position) Subscribed {
	body := map[string]any{"channel": channel}
	if since != nil {
		body["recover"], body["offset"], body["epoch"] = true, since.Offset, since.Epoch
	}
	r := c.Call(map[string]any{"subscribe": body})
	if r.Error != nil {
		return Subscribed{Err: r.Error}
	}
	var result struct {
		Recovered    bool   `json:"recovered"`
		Offset       uint64 `json:"offset"`
		Epoch        string `json:"epoch"`
		Publications []struct {
			Offset uint64          `json:"offset"`
			Data   json.RawMessage `json:"data"`
		} `json:"publications"`
	}
	_ = json.Unmarshal(r.Raw["subscribe"], &result)
	out := Subscribed{Recovered: result.Recovered, Offset: result.Offset, Epoch: result.Epoch}
	for _, pub := range result.Publications {
		out.Pubs = append(out.Pubs, Push{Channel: channel, Offset: pub.Offset, Data: pub.Data})
	}
	return out
}

// RPC calls a method with a JSON body and gives the reply's data, or the error.
func (c *Client) RPC(method string, body any) (json.RawMessage, *Error) {
	data, _ := json.Marshal(body)
	r := c.Call(map[string]any{"rpc": map[string]any{"method": method, "data": json.RawMessage(data)}})
	if r.Error != nil {
		return nil, r.Error
	}
	var wrapped struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(r.Raw["rpc"], &wrapped)
	return wrapped.Data, nil
}

// Next waits for the next push on any channel.
func (c *Client) Next(within time.Duration) (Push, bool) {
	select {
	case p := <-c.Pushes:
		return p, true
	case <-time.After(within):
		return Push{}, false
	case <-c.Closed:
		select {
		case p := <-c.Pushes:
			return p, true
		default:
			return Push{}, false
		}
	}
}

// Close closes the socket and waits for the reader to end.
func (c *Client) Close() {
	_ = c.ws.Close()
	<-c.Closed
}
