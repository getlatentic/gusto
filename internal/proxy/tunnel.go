package proxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/spend"
)

// dialLimit is how long the proxy waits for an allowed host to accept.
const dialLimit = 15 * time.Second

// tunnel carries an HTTPS connection to an allowed host. The proxy sees the
// host and none of what was said: the TLS is between the container and the
// host.
//
// The line is written when the tunnel opens, before the container hears it
// may go ahead, rather than when it closes. A connection can outlive the
// harness that opened it, and a line written after the manifest hashed the
// record would read as evidence changed after the run.
func (p *Proxy) tunnel(w http.ResponseWriter, job, address string, call spend.Call) {
	sent := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), dialLimit)
	defer cancel()
	remote, err := p.dial(ctx, "tcp", address)
	if err != nil {
		call.Error, call.Took = err.Error(), since(sent)
		p.note(job, call)
		http.Error(w, "that host did not answer", http.StatusBadGateway)
		return
	}
	defer remote.Close()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "this connection cannot be tunnelled", http.StatusInternalServerError)
		return
	}
	call.Status, call.Took = http.StatusOK, since(sent)
	p.note(job, call)
	local, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	if _, err := local.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		local.Close()
		return
	}
	Splice(withBuffered(local, buffered.Reader), remote)
}

// Splice copies both ways between two connections until either side is done,
// then closes both, and says how much went each way.
func Splice(near, far io.ReadWriteCloser) (sent, received int64) {
	var both sync.WaitGroup
	both.Add(2)
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			near.Close()
			far.Close()
		})
	}
	go func() {
		defer both.Done()
		sent, _ = io.Copy(far, near)
		closeBoth()
	}()
	go func() {
		defer both.Done()
		received, _ = io.Copy(near, far)
		closeBoth()
	}()
	both.Wait()
	return sent, received
}

// buffered is a hijacked connection whose first bytes may already sit in the
// server's reader. Reading them first keeps a client that spoke early whole.
type buffered struct {
	net.Conn
	reader io.Reader
}

func (b buffered) Read(into []byte) (int, error) { return b.reader.Read(into) }

func withBuffered(conn net.Conn, reader *bufio.Reader) buffered {
	return buffered{Conn: conn, reader: io.MultiReader(io.LimitReader(reader, int64(reader.Buffered())), conn)}
}
