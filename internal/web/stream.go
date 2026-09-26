package web

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/getlatentic/haas/internal/jobs"
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
	flusher, streams := w.(http.Flusher)
	if !streams {
		http.Error(w, "this connection cannot stream", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	tail := &tail{path: dir.OutFile("transcript.log")}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, line := range tail.lines() {
			fmt.Fprintf(w, "data: %s\n\n", line)
		}
		flusher.Flush()
		if done, state := s.ended(record.ID); done {
			// One more read, because the last bytes can land after the state does.
			for _, line := range tail.lines() {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
			fmt.Fprintf(w, "event: ended\ndata: %s\n\n", state)
			flusher.Flush()
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// ended asks the index rather than the file, because a job that wrote nothing
// still ends.
func (s *Server) ended(id string) (bool, jobs.State) {
	record, found := s.index.Get(id)
	if !found {
		return true, jobs.Interrupted
	}
	return record.State.Done(), record.State
}

// tail reads a file that is still being written, and hands back whole lines.
type tail struct {
	path      string
	at        int64
	remainder []byte
}

func (t *tail) lines() []string {
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
