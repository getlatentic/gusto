package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAServerIsNotBrokenForTheTaskByAFirstRequestThatWasCancelled(t *testing.T) {
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	h.session.catalog.load(gone)
	if found, _ := h.call("tool_search", map[string]any{"query": "report"}); !strings.Contains(found, "reports/get_report") {
		t.Errorf("a server read first by a request that was cancelled stays broken: %q", found)
	}
}

func TestALostSessionEndsOnlyThatSession(t *testing.T) {
	fake, server := reports(t)
	conn := DialWith(server, http.DefaultClient)
	defer conn.Close()
	if _, err := conn.Tools(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.gone = true
	// Another call starts a new session while this one learns its own is gone.
	fake.lost = func(u *upstream) {
		u.session++
		conn.mu.Lock()
		conn.session, conn.ready = "session-2", true
		conn.mu.Unlock()
	}
	fake.mu.Unlock()
	if _, err := conn.Call(context.Background(), "list_reports", nil); err != nil {
		t.Fatal(err)
	}
	if fake.inits != 1 {
		t.Errorf("the session another call had started was ended and a third begun: %d starts", fake.inits)
	}
}

func TestAServerThatCouldNotBeReadIsReadAgainLater(t *testing.T) {
	fake, server := reports(t)
	fake.down = true
	h := start(t, Config{Servers: []Server{server}})
	if found, _ := h.call("tool_search", map[string]any{"query": "report"}); !strings.Contains(found, "could not be reached") {
		t.Fatalf("a server that is down was not said to be: %q", found)
	}
	fake.mu.Lock()
	fake.down = false
	fake.mu.Unlock()
	if found, _ := h.call("tool_search", map[string]any{"query": "report"}); strings.Contains(found, "reports/get_report") {
		t.Errorf("a server was read again at once: %q", found)
	}
	later := time.Now().Add(retryAfter)
	h.session.catalog.mu.Lock()
	h.session.catalog.now = func() time.Time { return later }
	h.session.catalog.mu.Unlock()
	if found, _ := h.call("tool_search", map[string]any{"query": "report"}); !strings.Contains(found, "reports/get_report") || strings.Contains(found, "could not be reached") {
		t.Errorf("a server that came up was not read again: %q", found)
	}
}
