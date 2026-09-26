package accesslog

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A stream behind the log must still reach its watcher frame by frame, and its
// line is written only once it ends.
func TestAStreamStillFlushesAndIsLoggedWhenItEnds(t *testing.T) {
	finish := make(chan struct{})
	deadline := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, streams := w.(http.Flusher); !streams {
			t.Error("the wrapped writer cannot flush")
			return
		}
		control := http.NewResponseController(w)
		deadline <- control.SetWriteDeadline(time.Now().Add(10 * time.Second))
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: first\n\n"))
		if err := control.Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		<-finish
		w.Write([]byte("data: last\n\n"))
	})
	wrapped, destination := kept(handler)
	server := httptest.NewServer(wrapped)
	defer server.Close()

	response, err := http.Get(server.URL + "/watch/1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if err := <-deadline; err != nil {
		t.Errorf("the write deadline did not reach the connection: %v", err)
	}
	lines := bufio.NewReader(response.Body)
	first, err := lines.ReadString('\n')
	if err != nil || first != "data: first\n" {
		t.Fatalf("the first frame did not arrive before the stream ended: %q, %v", first, err)
	}
	if logged := destination.String(); logged != "" {
		t.Errorf("a stream still open was logged: %s", logged)
	}

	close(finish)
	lines.ReadString('\n') // the blank line after the first frame
	if last, _ := lines.ReadString('\n'); last != "data: last\n" {
		t.Errorf("want the last frame, got %q", last)
	}
	logged := waitForLine(t, destination)
	if logged["path"] != "/watch/1" || logged["bytes"] != 25.0 {
		t.Errorf("the line does not describe the stream: %v", logged)
	}
}

// The handler returns a moment after its last frame reaches the client, so the
// test waits a little for the line rather than racing it.
func waitForLine(t *testing.T, destination *journal) map[string]any {
	t.Helper()
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); time.Sleep(10 * time.Millisecond) {
		if lines := destination.lines(t); len(lines) == 1 {
			return lines[0]
		}
	}
	t.Fatalf("no line after the stream ended: %s", destination)
	return nil
}

// Without Unwrap, http.ResponseController cannot see past the recorder and says
// a deadline is not supported, which stream.go reads as a writer with nothing
// to stall.
func TestTheRealWriterIsWithinReach(t *testing.T) {
	inner := httptest.NewRecorder()
	outer := &recorder{ResponseWriter: inner}
	if outer.Unwrap() != inner {
		t.Error("Unwrap does not hand back the real writer")
	}
	if err := http.NewResponseController(outer).Flush(); err != nil {
		t.Errorf("flush through the recorder: %v", err)
	}
	if !inner.Flushed {
		t.Error("the flush did not reach the real writer")
	}
}
