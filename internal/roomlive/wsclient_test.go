package roomlive

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roomlive/livetest"
)

// The tests speak to the server through the wire a browser uses, with the small client in livetest.

type (
	push       = livetest.Push
	position   = livetest.Position
	subscribed = livetest.Subscribed
	replyError = livetest.Error
)

type client struct{ *livetest.Client }

// connectClient opens a socket as a person and connects to the protocol.
func connectClient(t testing.TB, base, path, who string, stalled bool) (*client, int, error) {
	t.Helper()
	header := http.Header{}
	if who != "" {
		header.Set("X-Who", who)
	}
	c, status, err := livetest.Connect(t, base, path, header, stalled)
	if err != nil {
		return nil, status, err
	}
	return &client{c}, status, nil
}

func (c *client) subscribe(channel string, since *position) subscribed {
	c.T.Helper()
	return c.Subscribe(channel, since)
}

// rpc calls a command and decodes the Result.
func (c *client) rpc(method string, command Command) (Result, *replyError) {
	c.T.Helper()
	data, err := c.RPC(method, command)
	if err != nil {
		return Result{}, err
	}
	var result Result
	_ = json.Unmarshal(data, &result)
	return result, nil
}

func (c *client) next(within time.Duration) (push, bool) { return c.Next(within) }

// frames reads count frames from the stream, failing the test if they do not arrive.
func (c *client) frames(count int, within time.Duration) []Frame {
	c.T.Helper()
	var got []Frame
	deadline := time.Now().Add(within)
	for len(got) < count {
		p, ok := c.Next(time.Until(deadline))
		if !ok {
			c.T.Fatalf("wanted %d frames, got %d (last sequence %d)", count, len(got), lastSeq(got))
		}
		if !strings.HasPrefix(p.Channel, streamPrefix) || p.Data == nil {
			continue
		}
		var f Frame
		if err := json.Unmarshal(p.Data, &f); err != nil {
			c.T.Fatalf("bad frame %.200s", p.Data)
		}
		got = append(got, f)
	}
	return got
}

func lastSeq(frames []Frame) uint64 {
	if len(frames) == 0 {
		return 0
	}
	return frames[len(frames)-1].Seq
}

func (c *client) close() { c.Close() }

// dialWS opens a raw socket, for the tests of how an upgrade is refused.
func dialWS(base, path string, header http.Header) (*livetest.Conn, int, error) {
	return livetest.DialWS(base, path, header)
}
