package livetest

import (
	"bufio"
	"net"
	"testing"
	"time"
)

// A client closed while its reader is stalled still ends, so a test that fails while a client is stalled does not hang.
func TestAStalledClientCloses(t *testing.T) {
	server, mine := net.Pipe()
	defer server.Close()
	c := &Client{T: t, ws: &Conn{conn: mine, rd: bufio.NewReader(mine)}, wait: map[uint64]chan Answer{}, Pushes: make(chan Push, 1), Closed: make(chan struct{})}
	c.Stall(true)
	go c.read()
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("closing a stalled client hung")
	}
}
