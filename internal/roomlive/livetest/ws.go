// Package livetest is a WebSocket client small enough to read in full, speaking Centrifuge's JSON protocol, so that tests and
// load runs exercise the live layer through the wire a browser uses. It reads a frame at a time, answers the server's
// application pings, and hands pushes to a channel.
package livetest

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Conn is a WebSocket connection, framed by hand.
type Conn struct {
	conn net.Conn
	rd   *bufio.Reader
	wmu  sync.Mutex
}

// DialWS opens a socket to the server at base (an http URL) and path, with extra request headers, speaking the Centrifuge JSON
// subprotocol. A request with no Origin header gets one naming base, since a browser always sends one; give "X-No-Origin" to
// send none. It returns the response status, and the socket when the upgrade succeeded.
func DialWS(base, path string, header http.Header) (*Conn, int, error) {
	target, err := url.Parse(base)
	if err != nil {
		return nil, 0, err
	}
	conn, err := net.DialTimeout("tcp", target.Host, 5*time.Second)
	if err != nil {
		return nil, 0, err
	}
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: centrifuge-json\r\n",
		path, target.Host, base64.StdEncoding.EncodeToString(key))
	if header.Get("Origin") == "" && header.Get("X-No-Origin") == "" {
		request += "Origin: " + base + "\r\n"
	}
	for name, values := range header {
		if name == "X-No-Origin" {
			continue
		}
		for _, value := range values {
			request += name + ": " + value + "\r\n"
		}
	}
	if _, err := conn.Write([]byte(request + "\r\n")); err != nil {
		conn.Close()
		return nil, 0, err
	}
	rd := bufio.NewReaderSize(conn, 16<<10)
	response, err := http.ReadResponse(rd, nil)
	if err != nil {
		conn.Close()
		return nil, 0, err
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		_, _ = io.Copy(io.Discard, response.Body)
		conn.Close()
		return nil, response.StatusCode, errors.New(response.Status)
	}
	return &Conn{conn: conn, rd: rd}, response.StatusCode, nil
}

// ReadFrame reads one frame, giving its opcode and payload.
func (w *Conn) ReadFrame() (opcode byte, payload []byte, err error) {
	var head [2]byte
	if _, err = io.ReadFull(w.rd, head[:]); err != nil {
		return 0, nil, err
	}
	opcode = head[0] & 0x0f
	length := uint64(head[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(w.rd, ext[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(w.rd, ext[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	payload = make([]byte, length)
	_, err = io.ReadFull(w.rd, payload)
	return opcode, payload, err
}

// WriteFrame writes one masked frame, as a client must.
func (w *Conn) WriteFrame(opcode byte, payload []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	var out bytes.Buffer
	out.WriteByte(0x80 | opcode)
	switch n := len(payload); {
	case n < 126:
		out.WriteByte(0x80 | byte(n))
	case n < 1<<16:
		out.WriteByte(0x80 | 126)
		_ = binary.Write(&out, binary.BigEndian, uint16(n))
	default:
		out.WriteByte(0x80 | 127)
		_ = binary.Write(&out, binary.BigEndian, uint64(n))
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	out.Write(mask)
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	out.Write(masked)
	_ = w.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := w.conn.Write(out.Bytes())
	return err
}

// Close closes the connection without the closing handshake.
func (w *Conn) Close() error { return w.conn.Close() }
