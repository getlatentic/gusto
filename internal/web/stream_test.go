package web

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// A watcher here is served over a pipe rather than a socket. A write to a pipe
// finishes only once the reader takes it, so a client that stops reading stalls
// the handler on its next frame instead of a few hundred kilobytes later, once
// socket buffers whose size the test would have to guess are full. Everything
// above the connection is the real thing: a real http.Server holding a real
// request, which is what a stalled write holds on to.
type viewer struct {
	server   *http.Server
	conn     net.Conn
	response *http.Response
	letGo    <-chan struct{}
}

func watchOverAPipe(t *testing.T, site *site, id string) *viewer {
	t.Helper()
	letGo := make(chan struct{})
	var once sync.Once
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer once.Do(func() { close(letGo) })
		site.handler.ServeHTTP(w, r)
	})}
	client, served := net.Pipe()
	go server.Serve(&onePipe{conn: served, closed: make(chan struct{})})
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	head := "GET /watch/" + id + " HTTP/1.1\r\nHost: haas\r\nCookie: " +
		site.cookie.Name + "=" + site.cookie.Value + "\r\n\r\n"
	if _, err := client.Write([]byte(head)); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("the stream did not open: %s %q", response.Status, response.Header.Get("Content-Type"))
	}
	return &viewer{server: server, conn: client, response: response, letGo: letGo}
}

// onePipe serves one connection and then waits to be closed.
type onePipe struct {
	conn   net.Conn
	once   sync.Once
	closed chan struct{}
}

func (l *onePipe) Accept() (net.Conn, error) {
	if conn := l.conn; conn != nil {
		l.conn = nil
		return conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *onePipe) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *onePipe) Addr() net.Addr { return pipeAddr{} }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// transcriptLine is what the harness is taken to have said, long enough that a
// few thousand lines are more than one read of the file.
func transcriptLine(n int) string {
	return fmt.Sprintf("line %04d %s", n, strings.Repeat("x", 48))
}

// aJobThatSaid runs a job through the queue and puts lines in its transcript,
// which is what a watcher reads.
func aJobThatSaid(t *testing.T, site *site, lines int) string {
	t.Helper()
	answer := site.ask(t, "POST", "/jobs", url.Values{
		"repo": {"/repo"}, "brief": {"watch me"}, "gate": {"true"}, "harness": {"fixture"},
	})
	id := strings.TrimPrefix(answer.Header().Get("Location"), "/jobs/")
	select {
	case <-site.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the job never ran")
	}
	record, found := site.index.Get(id)
	if !found {
		t.Fatalf("no job called %q", id)
	}
	var said strings.Builder
	for n := range lines {
		said.WriteString(transcriptLine(n))
		said.WriteByte('\n')
	}
	path := record.Dir(site.index.Root()).OutFile("transcript.log")
	if err := os.WriteFile(path, []byte(said.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAWatcherThatStopsReadingIsHungUpOn(t *testing.T) {
	was := patience
	patience = 200 * time.Millisecond
	t.Cleanup(func() { patience = was })

	site := open(t)
	site.signIn(t)
	id := aJobThatSaid(t, site, 5000)

	// The head is read and the body never is: the frames have nowhere to go. The
	// read deadline is set while the connection is open, so a connection left
	// open below ends this test rather than hanging it.
	viewer := watchOverAPipe(t, site, id)
	if err := viewer.conn.SetReadDeadline(time.Now().Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	waited := time.Now()
	select {
	case <-viewer.letGo:
		t.Logf("the handler let go after %s", time.Since(waited).Round(time.Millisecond))
	case <-time.After(10 * time.Second):
		t.Fatal("the handler is still writing to a watcher that stopped reading")
	}

	// Hung up on, not merely given up on: the connection is closed, so the
	// watcher finds out rather than holding one open.
	_, err := io.ReadAll(viewer.response.Body)
	if err == nil {
		t.Fatal("the body ended cleanly, so the watcher was not hung up on")
	}
	var late net.Error
	if errors.As(err, &late) && late.Timeout() {
		t.Fatalf("the connection was left open: %v", err)
	}
	t.Logf("the connection ended with %v", err)

	// A stalled write is held by the server too, and Shutdown waits for every
	// handler it has out.
	stopping, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := viewer.server.Shutdown(stopping); err != nil {
		t.Fatalf("a stalled watcher held the server's shutdown open: %v", err)
	}
}

func TestAWatcherThatKeepsReadingSeesEveryLineAndTheVerdict(t *testing.T) {
	const lines = 5000
	site := open(t)
	site.signIn(t)
	id := aJobThatSaid(t, site, lines)

	// No patience is borrowed here: a watcher that reads is held to the same
	// deadline the server ships with.
	viewer := watchOverAPipe(t, site, id)
	if err := viewer.conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	seen, ended := 0, false
	frames := bufio.NewScanner(viewer.response.Body)
	for frames.Scan() {
		switch text := frames.Text(); {
		case strings.HasPrefix(text, "data: line "):
			if want := "data: " + transcriptLine(seen); text != want {
				t.Fatalf("frame %d is %q, and the order says %q", seen, text, want)
			}
			seen++
		case text == "event: ended":
			ended = true
		}
	}
	if err := frames.Err(); err != nil {
		t.Fatalf("a watcher that reads was cut off: %v", err)
	}
	if seen != lines {
		t.Errorf("%d of %d lines arrived", seen, lines)
	}
	if !ended {
		t.Error("the verdict never arrived")
	}
	select {
	case <-viewer.letGo:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler never let go of a finished stream")
	}
}
