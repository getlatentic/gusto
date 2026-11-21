package cloud

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Host runs many organizations' servers inside one process, for the shared tier: each is the same server a process of its own
// would be, built from the same environment and listening on its own ports, so routing, sign-in and every store stay apart by
// organization as they do with a process each. What the organizations share is the process: its memory, its Go runtime and the
// environment it was started with, which is the operator's (Controller.Env) and nothing of gusto cloud's own.
//
// gusto cloud asks it over a socket only it can open (HostedNode).
type Host struct {
	// Serve runs one organization's server, with env as its whole environment, until ctx ends, and says why it ended early.
	Serve func(ctx context.Context, env []string) error

	mu      sync.Mutex
	running map[string]*hostedServer
}

type hostedServer struct {
	stop context.CancelFunc
	done chan struct{}
}

// Handler is what the socket serves: POST /servers starts the server a ServerSpec describes, GET /servers is the organizations
// whose servers run, GET /servers/{slug} answers 200 while that one runs, and DELETE /servers/{slug}?grace=<seconds> stops it.
func (h *Host) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /servers", h.startAsked)
	mux.HandleFunc("GET /servers", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, h.Running()) })
	mux.HandleFunc("GET /servers/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if !h.Alive(r.PathValue("slug")) {
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("DELETE /servers/{slug}", func(w http.ResponseWriter, r *http.Request) {
		seconds, _ := strconv.Atoi(r.URL.Query().Get("grace"))
		h.Stop(r.PathValue("slug"), time.Duration(max(seconds, 1))*time.Second)
	})
	return mux
}

func (h *Host) startAsked(w http.ResponseWriter, r *http.Request) {
	var spec ServerSpec
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&spec); err != nil || ValidSlug(spec.Slug) != nil {
		http.Error(w, "the request is not a server to start", http.StatusBadRequest)
		return
	}
	h.Start(spec)
}

// Start runs the organization's server, and does nothing when it is already running.
func (h *Host) Start(spec ServerSpec) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if held, running := h.running[spec.Slug]; running && !closed(held.done) {
		return
	}
	if h.running == nil {
		h.running = map[string]*hostedServer{}
	}
	ctx, stop := context.WithCancel(context.Background())
	held := &hostedServer{stop: stop, done: make(chan struct{})}
	h.running[spec.Slug] = held
	go func() {
		defer close(held.done)
		defer stop()
		if err := h.Serve(ctx, spec.Env); err != nil {
			slog.Error("an organization's server ended", "org", spec.Slug, "error", err)
		}
	}()
}

// Alive says the organization's server is running.
func (h *Host) Alive(slug string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	held, running := h.running[slug]
	return running && !closed(held.done)
}

// Running is the organizations whose servers are running, in order.
func (h *Host) Running() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	slugs := []string{}
	for slug, held := range h.running {
		if !closed(held.done) {
			slugs = append(slugs, slug)
		}
	}
	slices.Sort(slugs)
	return slugs
}

// Stop asks the organization's server to end and waits up to grace for it. A goroutine cannot be killed, so a server that has
// not ended by then is left to end on its own, and is said.
func (h *Host) Stop(slug string, grace time.Duration) {
	h.mu.Lock()
	held, running := h.running[slug]
	h.mu.Unlock()
	if !running {
		return
	}
	held.stop()
	select {
	case <-held.done:
	case <-time.After(grace):
		slog.Error("an organization's server did not end in time", "org", slug, "grace", grace)
	}
}

// StopAll stops every server at once and waits up to grace for them all.
func (h *Host) StopAll(grace time.Duration) {
	var stopping sync.WaitGroup
	for _, slug := range h.Running() {
		stopping.Go(func() { h.Stop(slug, grace) })
	}
	stopping.Wait()
}

func closed(done chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
