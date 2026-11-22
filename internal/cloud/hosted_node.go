package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// hostAsk bounds one ask of the host, but for a stop, which is given its grace on top.
const hostAsk = 10 * time.Second

// hostStartWait is how long a host that was just started has to open its socket.
const hostStartWait = 15 * time.Second

// HostedNode is this machine, running every organization's server inside one process, the host (Host), which it starts as its
// child and starts again when it ends. A host that ends takes every server with it, and the controller starts each again, as it
// does a child that ended.
type HostedNode struct {
	// Command is the host's program and arguments, and Env its whole environment: the operator's settings, as every
	// organization's server has, and nothing of gusto cloud's own.
	Command []string
	Env     []string
	// Socket is where the host listens and Log where its output and every server's goes.
	Socket, Log string

	// starting is held while the host is started, so two servers that start at once start one host. mu guards the fields
	// below and is never held across a wait.
	starting sync.Mutex
	mu       sync.Mutex
	process  *exec.Cmd
	ended    chan struct{}
	// started is the environment each server was started with, which says what to clean up after it.
	started map[string][]string
}

func (h *HostedNode) Name() string   { return "shared" }
func (h *HostedNode) Host() string   { return "127.0.0.1" }
func (h *HostedNode) Listen() string { return "127.0.0.1" }

func (h *HostedNode) Ports(from int, taken map[int]bool) (server, proxy int, err error) {
	return freePorts(from, taken)
}

func (h *HostedNode) Held(port int) bool { return !free(port) }

func (h *HostedNode) Start(spec ServerSpec) error {
	if h.Alive(spec.Slug) {
		return nil
	}
	removeLeftovers(spec.Slug, spec.Env)
	if err := h.ensureHost(); err != nil {
		return err
	}
	body, err := json.Marshal(ServerSpec{Slug: spec.Slug, Env: spec.Env})
	if err != nil {
		return err
	}
	if _, err := h.ask(http.MethodPost, "/servers", body, hostAsk, nil); err != nil {
		return fmt.Errorf("the host did not start %s's server: %w", spec.Slug, err)
	}
	h.mu.Lock()
	if h.started == nil {
		h.started = map[string][]string{}
	}
	h.started[spec.Slug] = spec.Env
	h.mu.Unlock()
	return nil
}

func (h *HostedNode) Stop(slug string, grace time.Duration) error {
	h.mu.Lock()
	env := h.started[slug]
	delete(h.started, slug)
	h.mu.Unlock()
	if h.hostRunning() {
		query := "/servers/" + url.PathEscape(slug) + "?grace=" + strconv.Itoa(int(grace.Seconds()))
		if _, err := h.ask(http.MethodDelete, query, nil, grace+hostAsk, nil); err != nil {
			return err
		}
	}
	removeLeftovers(slug, env)
	return nil
}

func (h *HostedNode) Alive(slug string) bool {
	if !h.hostRunning() {
		return false
	}
	status, err := h.ask(http.MethodGet, "/servers/"+url.PathEscape(slug), nil, hostAsk, nil)
	return err == nil && status == http.StatusOK
}

// Running is the organizations whose servers are running in the host.
func (h *HostedNode) Running() []string {
	if !h.hostRunning() {
		return nil
	}
	var slugs []string
	if _, err := h.ask(http.MethodGet, "/servers", nil, hostAsk, &slugs); err != nil {
		return nil
	}
	return slugs
}

// ensureHost starts the host when it is not running, and waits for its socket to answer.
func (h *HostedNode) ensureHost() error {
	h.starting.Lock()
	defer h.starting.Unlock()
	if h.hostRunning() {
		return nil
	}
	if len(h.Command) == 0 {
		return errors.New("no command was given for the host")
	}
	log, err := os.OpenFile(h.Log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	// A socket left by a host that ended is not one that answers, and the new host could not listen where it is.
	_ = os.Remove(h.Socket)
	command := exec.Command(h.Command[0], h.Command[1:]...)
	command.Env = h.Env
	command.Stdout, command.Stderr = log, log
	command.SysProcAttr = dieWithParent()
	if err := command.Start(); err != nil {
		log.Close()
		return err
	}
	ended := make(chan struct{})
	go func() {
		command.Wait()
		log.Close()
		close(ended)
	}()
	h.mu.Lock()
	h.process, h.ended = command, ended
	h.mu.Unlock()
	return h.waitForSocket(ended)
}

func (h *HostedNode) waitForSocket(ended chan struct{}) error {
	deadline := time.Now().Add(hostStartWait)
	for time.Now().Before(deadline) {
		if closed(ended) {
			return fmt.Errorf("the host ended as it started; its output is in %s", h.Log)
		}
		if _, err := h.ask(http.MethodGet, "/servers", nil, time.Second, nil); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the host did not open %s within %s", h.Socket, hostStartWait)
}

func (h *HostedNode) hostRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.process != nil && !closed(h.ended)
}

// ask sends one request to the host and is its status, or why there is none, with the answer read into into when it is not nil.
// A status other than 2xx or 404 is an error.
func (h *HostedNode) ask(method, path string, body []byte, within time.Duration, into any) (int, error) {
	request, err := http.NewRequest(method, "http://host"+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	response, err := h.client(within).Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
		return response.StatusCode, fmt.Errorf("the host answered %s", response.Status)
	}
	if into != nil {
		return response.StatusCode, json.NewDecoder(response.Body).Decode(into)
	}
	return response.StatusCode, nil
}

func (h *HostedNode) client(within time.Duration) *http.Client {
	return &http.Client{
		Timeout: within,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", h.Socket)
			},
			DisableKeepAlives: true,
		},
	}
}
