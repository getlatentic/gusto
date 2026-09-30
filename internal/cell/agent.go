package cell

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/cloud"
)

// Agent is what runs on a cell host. It starts and stops the organisations' servers the front door
// asks for, on this machine, and says what room there is. It does what a signed request says and
// nothing else, so the secret is what stands between the network and a command run here.
type Agent struct {
	Name    string
	Private string
	Secret  []byte
	Servers *cloud.LocalCell
	// Measure says what the machine has left. It is Measure unless a test says otherwise.
	Measure func() Capacity
	now     func() time.Time
}

// Status is what a cell says of itself.
type Status struct {
	Name    string   `json:"name"`
	Private string   `json:"private"`
	Servers []string `json:"servers"`
	Capacity
}

// startRequest is what the front door sends to start a server.
type startRequest struct {
	Slug    string   `json:"slug"`
	Command []string `json:"command"`
	Env     []string `json:"env"`
	Log     string   `json:"log"`
}

type stopRequest struct {
	GraceSeconds int `json:"graceSeconds"`
}

func (a *Agent) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// Handler is the agent's requests.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/servers", a.start)
	mux.HandleFunc("POST /v1/servers/{slug}/stop", a.stop)
	mux.HandleFunc("GET /v1/servers/{slug}", a.alive)
	mux.HandleFunc("GET /v1/ports", a.ports)
	mux.HandleFunc("GET /v1/status", a.status)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verify(r, a.Secret, a.clock()); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func (a *Agent) start(w http.ResponseWriter, r *http.Request) {
	var asked startRequest
	if err := json.NewDecoder(r.Body).Decode(&asked); err != nil || cloud.ValidSlug(asked.Slug) != nil || len(asked.Command) == 0 || asked.Log == "" {
		http.Error(w, "a slug, a command and a log are what starting a server needs", http.StatusBadRequest)
		return
	}
	if err := a.Servers.Start(cloud.ServerSpec{Slug: asked.Slug, Command: asked.Command, Env: asked.Env, Log: asked.Log}); err != nil {
		slog.Error("a server could not be started", "org", asked.Slug, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reply(w, http.StatusOK, map[string]bool{"started": true})
}

func (a *Agent) stop(w http.ResponseWriter, r *http.Request) {
	var asked stopRequest
	json.NewDecoder(r.Body).Decode(&asked)
	if cloud.ValidSlug(r.PathValue("slug")) != nil {
		http.Error(w, "that is not an organisation", http.StatusBadRequest)
		return
	}
	a.Servers.Stop(r.PathValue("slug"), time.Duration(max(asked.GraceSeconds, 1))*time.Second)
	reply(w, http.StatusOK, map[string]bool{"stopped": true})
}

func (a *Agent) alive(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, map[string]bool{"alive": a.Servers.Alive(r.PathValue("slug"))})
}

func (a *Agent) ports(w http.ResponseWriter, r *http.Request) {
	from, err := strconv.Atoi(r.URL.Query().Get("from"))
	if err != nil || from < 1024 || from > 60000 {
		http.Error(w, "from is the first port to try", http.StatusBadRequest)
		return
	}
	taken := map[int]bool{}
	for _, each := range strings.Split(r.URL.Query().Get("taken"), ",") {
		if port, err := strconv.Atoi(each); err == nil {
			taken[port] = true
		}
	}
	server, proxy, err := a.Servers.Ports(from, taken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	reply(w, http.StatusOK, map[string]int{"server": server, "proxy": proxy})
}

func (a *Agent) status(w http.ResponseWriter, r *http.Request) {
	measure := a.Measure
	if measure == nil {
		measure = Measure
	}
	reply(w, http.StatusOK, Status{Name: a.Name, Private: a.Private, Servers: a.Servers.Running(), Capacity: measure()})
}
