package helper

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryVault struct {
	mu     sync.Mutex
	tokens map[string]string
}

func (m *memoryVault) Get(server string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if token, ok := m.tokens[Server(server)]; ok {
		return token, nil
	}
	return "", ErrNotSignedIn
}
func (m *memoryVault) Set(server, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tokens == nil {
		m.tokens = map[string]string{}
	}
	m.tokens[Server(server)] = token
	return nil
}
func (m *memoryVault) Delete(server string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, Server(server))
	return nil
}

// organisation is a server that says who a token is and streams the events a test gives it.
type organisation struct {
	*httptest.Server
	mu      sync.Mutex
	opened  []string
	frames  chan string
	refused bool
}

func newOrganisation(t *testing.T) *organisation {
	t.Helper()
	o := &organisation{frames: make(chan string, 16)}
	o.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		refused := o.refused
		o.mu.Unlock()
		if refused || r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/me":
			json.NewEncoder(w).Encode(Who{Person: "ada@acme.example", Role: "runner"})
		case "/api/events":
			o.mu.Lock()
			o.opened = append(o.opened, r.URL.RawQuery)
			o.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			for {
				select {
				case frame, open := <-o.frames:
					if !open {
						return
					}
					if frame == "DROP" {
						return
					}
					w.Write([]byte(frame))
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(o.Close)
	return o
}

func (o *organisation) say(kind, job, state string, at int64) {
	body, _ := json.Marshal(Turn{Job: job, Brief: "a brief", State: state, At: at})
	o.frames <- "event: " + kind + "\ndata: " + string(body) + "\n\n"
}

func (o *organisation) queries() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.opened...)
}

// running starts a helper against the organisation and stops it when the test ends.
func running(t *testing.T, o *organisation, vault Vault, dir string) (*Daemon, Local, context.CancelFunc) {
	t.Helper()
	d := &Daemon{Server: o.URL, Vault: vault, Dir: dir, Retry: func(int) time.Duration { return 10 * time.Millisecond }, Signed: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	var local Local
	waitFor(t, "the helper to publish where it is", func() bool {
		found, err := Discover(dir)
		local = found
		return err == nil
	})
	return d, local, cancel
}

func waitFor(t *testing.T, what string, ready func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s did not happen", what)
}

func askStatus(local Local) Status {
	status, _ := local.Ask(context.Background())
	return status
}

func TestTheHelperHearsTheServerAndTellsEveryShellWhatItHeard(t *testing.T) {
	o := newOrganisation(t)
	vault := &memoryVault{}
	vault.Set(o.URL, "good")
	_, local, _ := running(t, o, vault, t.TempDir())
	waitFor(t, "the helper to be connected", func() bool { s := askStatus(local); return s.Connected && s.SignedIn && s.Person == "ada@acme.example" })

	o.say("turn.started", "j1", "running", 100)
	waitFor(t, "the turn to be heard", func() bool {
		request, _ := http.NewRequest("GET", "http://"+local.Address+"/v1/turns", nil)
		request.Header.Set("Authorization", "Bearer "+local.Token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		var turns []Turn
		json.NewDecoder(response.Body).Decode(&turns)
		return len(turns) == 1 && turns[0].State == "running"
	})

	// A shell that arrives later hears the turn, then ready, then what follows.
	request, _ := http.NewRequest("GET", "http://"+local.Address+"/v1/events", nil)
	request.Header.Set("Authorization", "Bearer "+local.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	lines := bufio.NewScanner(response.Body)
	next := func() string {
		var frame []string
		for lines.Scan() {
			if lines.Text() == "" {
				return strings.Join(frame, "|")
			}
			frame = append(frame, lines.Text())
		}
		t.Fatal("the stream ended")
		return ""
	}
	if first := next(); !strings.HasPrefix(first, "event: turn.started|") || !strings.Contains(first, `"job":"j1"`) {
		t.Fatalf("first %q", first)
	}
	if ready := next(); !strings.HasPrefix(ready, "event: ready") {
		t.Fatalf("ready %q", ready)
	}
	o.say("turn.finished", "j1", "passed", 200)
	if done := next(); !strings.HasPrefix(done, "event: turn.finished|") {
		t.Fatalf("done %q", done)
	}
}

func TestOnlyAProgramWithTheTokenAndTheRightAddressIsAnswered(t *testing.T) {
	o := newOrganisation(t)
	vault := &memoryVault{}
	vault.Set(o.URL, "good")
	_, local, _ := running(t, o, vault, t.TempDir())

	for name, test := range map[string]struct {
		host, token string
		want        int
	}{
		"no token":          {local.Address, "", http.StatusUnauthorized},
		"a wrong token":     {local.Address, "guess", http.StatusUnauthorized},
		"another name":      {"evil.example:" + portOf(local.Address), local.Token, http.StatusForbidden},
		"the right token":   {local.Address, local.Token, http.StatusOK},
		"localhost by name": {"localhost:" + portOf(local.Address), local.Token, http.StatusOK},
	} {
		request, _ := http.NewRequest("GET", "http://"+local.Address+"/v1/status", nil)
		request.Host = test.host
		if test.token != "" {
			request.Header.Set("Authorization", "Bearer "+test.token)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		response.Body.Close()
		if response.StatusCode != test.want {
			t.Errorf("%s answered %d, not %d", name, response.StatusCode, test.want)
		}
	}
}

func TestASecondHelperRefusesAndTheFirstLeavesNothingBehind(t *testing.T) {
	o := newOrganisation(t)
	vault := &memoryVault{}
	vault.Set(o.URL, "good")
	dir := t.TempDir()
	_, local, stop := running(t, o, vault, dir)

	second := &Daemon{Server: o.URL, Vault: vault, Dir: dir}
	if err := second.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("a second helper said %v", err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(StateFile(dir)); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("how to reach the helper is kept as %v, %v", info.Mode(), err)
		}
	}
	stop()
	waitFor(t, "the helper to take its file away", func() bool { _, err := os.Stat(StateFile(dir)); return err != nil })
	if _, err := local.Ask(context.Background()); err == nil {
		t.Error("a stopped helper still answered")
	}
}

func TestAConnectionThatDropsIsGotBackAndAsksToCatchUp(t *testing.T) {
	o := newOrganisation(t)
	vault := &memoryVault{}
	vault.Set(o.URL, "good")
	_, local, _ := running(t, o, vault, t.TempDir())
	waitFor(t, "the first connection", func() bool { return len(o.queries()) == 1 })
	if o.queries()[0] != "" {
		t.Errorf("a first connection asked to catch up from %q", o.queries()[0])
	}
	o.say("turn.started", "j1", "running", 1000)
	waitFor(t, "the turn", func() bool { return len(askTurns(t, local)) == 1 })
	o.frames <- "DROP"
	waitFor(t, "the second connection", func() bool { return len(o.queries()) == 2 })
	if got := o.queries()[1]; got != "since=940" {
		t.Errorf("the second connection asked %q, not to catch up from a minute before the last event heard", got)
	}
	waitFor(t, "the helper to be connected again", func() bool { return askStatus(local).Connected })
}

func askTurns(t *testing.T, local Local) []Turn {
	t.Helper()
	request, _ := http.NewRequest("GET", "http://"+local.Address+"/v1/turns", nil)
	request.Header.Set("Authorization", "Bearer "+local.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	var turns []Turn
	json.NewDecoder(response.Body).Decode(&turns)
	return turns
}

func TestAPersonWhoSignsInAfterTheHelperStartedNeedsNoRestart(t *testing.T) {
	o := newOrganisation(t)
	vault := &memoryVault{}
	_, local, _ := running(t, o, vault, t.TempDir())
	if status := askStatus(local); status.SignedIn || status.Connected {
		t.Fatalf("a computer with no token was %+v", status)
	}
	vault.Set(o.URL, "good")
	waitFor(t, "the helper to notice the sign-in", func() bool { s := askStatus(local); return s.SignedIn && s.Connected })
}

func TestATokenTheServerEndedMakesThePersonSignedOutAndTheHelperKeepsRunning(t *testing.T) {
	o := newOrganisation(t)
	vault := &memoryVault{}
	vault.Set(o.URL, "good")
	_, local, _ := running(t, o, vault, t.TempDir())
	waitFor(t, "the helper to be connected", func() bool { return askStatus(local).Connected })
	o.mu.Lock()
	o.refused = true
	o.mu.Unlock()
	o.frames <- "DROP"
	waitFor(t, "the helper to find it is signed out", func() bool { s := askStatus(local); return !s.SignedIn && !s.Connected })
}
