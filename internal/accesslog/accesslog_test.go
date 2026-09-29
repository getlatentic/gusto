package accesslog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// journal is a log destination a test can read while a server writes to it.
type journal struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (j *journal) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.text.Write(p)
}

func (j *journal) String() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.text.String()
}

// lines is every line written so far, decoded.
func (j *journal) lines(t *testing.T) []map[string]any {
	t.Helper()
	var decoded []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(j.String()), "\n") {
		if line == "" {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("a line that is not JSON: %q", line)
		}
		decoded = append(decoded, fields)
	}
	return decoded
}

func kept(handler http.Handler) (http.Handler, *journal) {
	destination := &journal{}
	return Keep(slog.New(slog.NewJSONHandler(destination, nil)))(handler), destination
}

// answer serves one request and returns the one line it left.
func answer(t *testing.T, handler http.Handler, r *http.Request) map[string]any {
	t.Helper()
	wrapped, destination := kept(handler)
	wrapped.ServeHTTP(httptest.NewRecorder(), r)
	lines := destination.lines(t)
	if len(lines) != 1 {
		t.Fatalf("want one line, got %d: %s", len(lines), destination)
	}
	return lines[0]
}

func TestALineSaysWhatWasAskedAndWhatWasAnswered(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("twelve bytes"))
	})
	line := answer(t, handler, httptest.NewRequest("POST", "/api/jobs", nil))
	want := map[string]any{"msg": "request", "method": "POST", "path": "/api/jobs", "status": 201.0, "bytes": 12.0, "caller": "none"}
	for field, value := range want {
		if line[field] != value {
			t.Errorf("%s: want %v, got %v", field, value, line[field])
		}
	}
	for _, field := range []string{"time", "duration_ms", "remote"} {
		if _, present := line[field]; !present {
			t.Errorf("no %s in %v", field, line)
		}
	}
}

func TestARefusalIsInTheStatus(t *testing.T) {
	refused := map[string]http.HandlerFunc{
		"guard": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/login", http.StatusSeeOther) },
		"sent":  func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
	}
	statuses := map[string]float64{"guard": 303, "sent": 401}
	for name, handler := range refused {
		line := answer(t, handler, httptest.NewRequest("GET", "/", nil))
		if line["status"] != statuses[name] {
			t.Errorf("%s: want %v, got %v", name, statuses[name], line["status"])
		}
	}
}

func TestAHandlerThatWritesNothingAnswers200(t *testing.T) {
	line := answer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), httptest.NewRequest("GET", "/", nil))
	if line["status"] != 200.0 || line["bytes"] != 0.0 {
		t.Errorf("want 200 and no bytes, got %v and %v", line["status"], line["bytes"])
	}
}

func TestHealthzIsNotLogged(t *testing.T) {
	wrapped, destination := kept(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) }))
	recorder := httptest.NewRecorder()
	wrapped.ServeHTTP(recorder, httptest.NewRequest("GET", "/healthz", nil))
	if recorder.Body.String() != "ok\n" {
		t.Errorf("the probe was not answered: %q", recorder.Body)
	}
	if destination.String() != "" {
		t.Errorf("a probe was logged: %s", destination)
	}
}

func TestProbesAndWorkingScrapesAreNotLoggedAndRefusedOnesAre(t *testing.T) {
	for path, status := range map[string]int{"/readyz": 200, "/metrics": 200} {
		wrapped, destination := kept(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if destination.String() != "" {
			t.Errorf("%s answering %d was logged: %s", path, status, destination)
		}
	}
	wrapped, destination := kept(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(destination.String(), `"path":"/metrics"`) || !strings.Contains(destination.String(), `"status":401`) {
		t.Errorf("a refused scrape was not logged: %q", destination.String())
	}
}
