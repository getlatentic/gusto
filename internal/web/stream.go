package web

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
)

// watch streams what the harness is saying, as it says it.
//
// The file is the stream: everybody watching reads the same bytes in the same
// order, a watcher who arrives late is caught up by starting at the beginning,
// and a job started from the command line in the same directory is watchable
// too. Nothing has to be told about anybody else.
func (s *Server) watch(w http.ResponseWriter, r *http.Request) {
	record, dir, found := s.find(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	if _, streams := w.(http.Flusher); !streams {
		http.Error(w, "this connection cannot stream", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	stream := &feed{to: w, control: http.NewResponseController(w)}
	stream.flush()

	tail := &tail{path: dir.OutFile("transcript.log"), plain: dir.PlainOut}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		stream.pour(tail)
		if stream.gone {
			return
		}
		if record = s.latest(record); record.State.Done() {
			// One more read, because the last bytes can land after the state does.
			stream.pour(tail)
			stream.send("event: ended\ndata: %s\n\n", record.State)
			stream.flush()
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// patience is how long one frame may wait for the watcher to take it. Following
// a job from a phone on a train is the point, so this is many times the gap
// between reads; what it rules out is a client that has stopped reading at all.
var patience = 10 * time.Second

// feed writes server-sent events to one watcher, and hangs up on a watcher that
// is not taking them.
//
// Falling behind costs a watcher nothing on its own: the transcript file is the
// backlog, and each watcher holds its own position in it. The cost is the frame.
// net/http gives a handler the connection, so a frame written to a client that
// has stopped draining its socket parks this goroutine with no deadline, and the
// request's context is read in a select the goroutine never reaches again — so
// nothing notices, and the goroutine, the connection, and any shutdown waiting
// on the handler are held until the process dies. A deadline per frame makes
// that a write error, and net/http closes a connection whose write failed as
// soon as the handler returns.
type feed struct {
	to      http.ResponseWriter
	control *http.ResponseController
	gone    bool
}

// pour sends every whole line the transcript has gained, so a job that said more
// than one read's worth is not cut off at the size of the buffer.
func (f *feed) pour(t *tail) {
	for !f.gone {
		lines := t.lines()
		if len(lines) == 0 {
			return
		}
		for _, line := range lines {
			f.send("data: %s\n\n", line)
		}
		f.flush()
	}
}

func (f *feed) send(format string, args ...any) {
	if !f.ready() {
		return
	}
	if _, err := fmt.Fprintf(f.to, format, args...); err != nil {
		f.gone = true
	}
}

// flush is where a watcher that stopped reading is found, because the frames
// before it fit in net/http's own buffer and only reach the client here.
func (f *feed) flush() {
	if !f.ready() {
		return
	}
	if err := f.control.Flush(); err != nil {
		f.gone = true
	}
}

// ready gives the client a fresh deadline for the write about to happen. A
// ResponseWriter with no deadline to give cannot stall a handler either: a test's
// recorder holds the whole body in memory.
func (f *feed) ready() bool {
	if f.gone {
		return false
	}
	err := f.control.SetWriteDeadline(time.Now().Add(patience))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		f.gone = true
	}
	return !f.gone
}

// latest asks the index rather than the file how the job stands, because a job
// that wrote nothing still ends. The index has no word on a job only when it
// never knew it, and then the record the watcher already holds is the newest
// thing known: it says no more than it did, and in particular no verdict.
func (s *Server) latest(known jobs.Record) jobs.Record {
	if now, found := s.index.Get(known.ID); found {
		return now
	}
	return known
}

// tail reads a file that is still being written, and hands back whole lines.
type tail struct {
	path string
	// plain says whether the file may be opened: it is a file and not a link.
	plain     func(name string) bool
	at        int64
	remainder []byte
}

func (t *tail) lines() []string {
	if t.plain != nil && !t.plain(filepath.Base(t.path)) {
		return nil
	}
	file, err := os.Open(t.path)
	if err != nil {
		return nil // the harness has not written anything yet
	}
	defer file.Close()
	if _, err := file.Seek(t.at, 0); err != nil {
		return nil
	}
	fresh := make([]byte, 64*1024)
	read, err := file.Read(fresh)
	if read <= 0 || (err != nil && read == 0) {
		return nil
	}
	t.at += int64(read)
	t.remainder = append(t.remainder, fresh[:read]...)
	var lines []string
	for {
		cut := bytes.IndexByte(t.remainder, '\n')
		if cut < 0 {
			break
		}
		lines = append(lines, string(t.remainder[:cut]))
		t.remainder = t.remainder[cut+1:]
	}
	return lines
}
