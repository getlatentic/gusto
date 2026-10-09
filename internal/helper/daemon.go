package helper

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Daemon is the helper running on a person's computer. It keeps one connection to their organisation's server, hears what
// happens to their jobs, and answers whatever on this computer asks, over a port only this computer reaches.
type Daemon struct {
	Server string
	Vault  Vault
	// Dir is where the file that says how to reach the helper is kept. It is the helper's settings folder.
	Dir string
	// Retry is how long to wait after the connection to the server was lost. A test replaces it.
	Retry func(attempt int) time.Duration
	// Signed is how long to wait before asking again when the person is signed out.
	Signed time.Duration

	Hub *Hub
	// Folders keeps folders on this computer the same as agents' folders in the cloud, when the helper was given a way to.
	Folders Folders

	mu        sync.Mutex
	person    string
	signedIn  bool
	connected bool
	tray      string
	token     string
	address   string
}

// Local is how a program on this computer reaches the running helper: the address it listens on and the token it asks for.
type Local struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Server  string `json:"server"`
}

// StateFile is where the running helper says how to reach it.
func StateFile(dir string) string { return filepath.Join(dir, "helper.json") }

// Discover reads how to reach the helper that is running.
func Discover(dir string) (Local, error) {
	text, err := os.ReadFile(StateFile(dir))
	if errors.Is(err, os.ErrNotExist) {
		return Local{}, errors.New("the helper is not running: start it with `haas-helper run`")
	}
	if err != nil {
		return Local{}, err
	}
	var found Local
	if err := json.Unmarshal(text, &found); err != nil || found.Address == "" {
		return Local{}, errors.New("the helper's settings are unreadable: remove " + StateFile(dir))
	}
	return found, nil
}

// Status is what the running helper says of itself.
type Status struct {
	Server    string `json:"server"`
	SignedIn  bool   `json:"signedIn"`
	Person    string `json:"person,omitempty"`
	Connected bool   `json:"connected"`
	// Tray says whether the icon is shown, and why not when it is not.
	Tray string `json:"tray,omitempty"`
}

// Ask asks the running helper for its status, which is also how a second one finds the first is there.
func (l Local) Ask(ctx context.Context) (Status, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+l.Address+"/v1/status", nil)
	if err != nil {
		return Status{}, err
	}
	request.Header.Set("Authorization", "Bearer "+l.Token)
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		return Status{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("the helper answered %d", response.StatusCode)
	}
	var status Status
	return status, json.NewDecoder(response.Body).Decode(&status)
}

// SetTray says whether the icon is shown ("shown", "off", or why there is none), which the status reports.
func (d *Daemon) SetTray(state string) { d.set(func() { d.tray = state }) }

// Status is how the helper stands now.
func (d *Daemon) Status() Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Status{Server: d.Server, SignedIn: d.signedIn, Person: d.person, Connected: d.connected, Tray: d.tray}
}

func (d *Daemon) set(update func()) {
	d.mu.Lock()
	update()
	d.mu.Unlock()
}

// Run serves until the context ends. It refuses to start when a helper is already answering.
func (d *Daemon) Run(ctx context.Context) error {
	if d.Hub == nil {
		d.Hub = NewHub()
	}
	if d.Retry == nil {
		d.Retry = Backoff
	}
	if d.Signed == 0 {
		d.Signed = 15 * time.Second
	}
	if running, err := Discover(d.Dir); err == nil {
		if _, err := running.Ask(ctx); err == nil {
			return fmt.Errorf("the helper is already running, on %s", running.Address)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	d.token, d.address = hex.EncodeToString(raw), listener.Addr().String()
	if err := d.publish(); err != nil {
		return err
	}
	defer os.Remove(StateFile(d.Dir))

	server := &http.Server{Handler: d.handler(), ReadHeaderTimeout: 10 * time.Second}
	go d.relay(ctx)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shut)
	case err := <-done:
		return err
	}
}

// publish writes how to reach the helper, readable by its owner alone, since the token in it is what lets a program ask.
func (d *Daemon) publish() error {
	if err := os.MkdirAll(d.Dir, 0o700); err != nil {
		return err
	}
	text, _ := json.Marshal(Local{Address: d.address, Token: d.token, PID: os.Getpid(), Server: d.Server})
	scratch := StateFile(d.Dir) + ".new"
	if err := os.WriteFile(scratch, text, 0o600); err != nil {
		return err
	}
	return os.Rename(scratch, StateFile(d.Dir))
}

// relay keeps the connection to the server, and gets it back after it is lost. A token it is not given, or the server will
// not take, is a person who is signed out: it waits and looks again, so signing in again needs no restart.
func (d *Daemon) relay(ctx context.Context) {
	attempt := 0
	for ctx.Err() == nil {
		token, err := d.Vault.Get(d.Server)
		if err != nil {
			d.set(func() { d.signedIn, d.connected, d.person = false, false, "" })
			d.pause(ctx, d.Signed)
			continue
		}
		client := Client{Server: d.Server, Token: token}
		who, err := client.Me(ctx)
		if errors.Is(err, ErrSignedOut) {
			d.set(func() { d.signedIn, d.connected, d.person = false, false, "" })
			d.pause(ctx, d.Signed)
			continue
		}
		if err != nil {
			d.set(func() { d.connected = false })
			d.pause(ctx, d.Retry(attempt))
			attempt++
			continue
		}
		d.set(func() { d.signedIn, d.person = true, who.Person })
		err = client.Follow(ctx, d.Hub, func(up bool) { d.set(func() { d.connected = up }); attempt = 0 })
		if errors.Is(err, ErrSignedOut) {
			d.set(func() { d.signedIn, d.connected, d.person = false, false, "" })
			d.pause(ctx, d.Signed)
			continue
		}
		d.pause(ctx, d.Retry(attempt))
		attempt++
	}
}

func (d *Daemon) pause(ctx context.Context, wait time.Duration) { _ = sleep(ctx, wait) }

// handler is what a program on this computer asks. Every request must come to the address the helper listens on and carry
// its token. The first keeps a web page from reaching it through a name that points at this computer, and the second keeps
// a page that guesses the port out.
func (d *Daemon) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, d.Status()) })
	mux.HandleFunc("GET /v1/turns", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, d.Hub.Turns()) })
	mux.HandleFunc("GET /v1/events", d.events)
	mux.HandleFunc("GET /v1/folders", d.listFolders)
	mux.HandleFunc("POST /v1/folders", d.addFolder)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != d.address && r.Host != "localhost:"+portOf(d.address) {
			http.Error(w, "that is not this helper's address", http.StatusForbidden)
			return
		}
		given, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(given), []byte(d.token)) != 1 {
			http.Error(w, "this needs the helper's token", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func portOf(address string) string {
	_, port, _ := net.SplitHostPort(address)
	return port
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// kinds are the words the server uses for a turn's state, so a shell hears the same events here as the server sends.
var kinds = map[string]string{
	"queued": "turn.queued", "running": "turn.started", "passed": "turn.finished", "failed": "turn.failed", "interrupted": "turn.interrupted",
}

// events streams what happens to the person's turns: the latest word on each, then `ready`, then each change.
func (d *Daemon) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "this connection cannot stream", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	live, stop := d.Hub.Listen()
	defer stop()
	send := func(kind string, turn any) bool {
		body, _ := json.Marshal(turn)
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, body)
		flusher.Flush()
		return err == nil
	}
	turns := d.Hub.Turns()
	for i := len(turns) - 1; i >= 0; i-- {
		if !send(kinds[turns[i].State], turns[i]) {
			return
		}
	}
	if !send("ready", struct{}{}) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-live:
			if !send(event.Kind, event.Turn) {
				return
			}
		}
	}
}
