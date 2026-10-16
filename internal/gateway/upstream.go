package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	mostAnswer = 8 << 20
	callTime   = 60 * time.Second
)

// Server is one MCP server the gateway calls on a job's behalf: an address, and the headers it is called with, which may be a
// credential. They are held here and never given to the job.
//
// A server may instead be a program: the gateway starts it, by the launcher it is given, and speaks to it over its standard input and
// output. Plugin is the folder of the plugin it came with, which it reads and may not change.
type Server struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Plugin  string            `json:"plugin,omitempty"`
	// Data is a folder the program may write, which belongs to this job and goes with it.
	Data string `json:"data,omitempty"`
}

// IsProgram says the server is a program and not an address.
func (s Server) IsProgram() bool { return s.Command != "" }

// Upstream is a connection to one Streamable HTTP server. It starts the session the first time it is used and starts it again
// when the server says the session is gone.
type Upstream struct {
	server Server
	client *http.Client
	ids    atomic.Int64

	mu      sync.Mutex
	session string
	ready   bool
}

// Dial makes the connection for a server. private lets the address be on a private network, which an administrator allows for a
// server inside the organisation; without it the address must be public, which is checked where the connection is made, so that a
// name that answers with a private address a moment later is not reached either.
func Dial(server Server, private bool) *Upstream {
	return &Upstream{server: server, client: &http.Client{
		Transport:     &http.Transport{DialContext: publicOnly(private), ResponseHeaderTimeout: callTime, MaxIdleConnsPerHost: 2},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// publicOnly is a dialer that refuses an address on a private network, whatever name it was reached by.
func publicOnly(private bool) func(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip != nil && !private && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
			return fmt.Errorf("%s is on a private network, which the gateway does not reach unless an administrator allows it", host)
		}
		return nil
	}}
	return dialer.DialContext
}

// Tools is every tool the server has, all its pages.
func (u *Upstream) Tools(ctx context.Context) ([]Tool, error) {
	return listTools(ctx, u.server.Name, u.call)
}

// Call calls one tool.
func (u *Upstream) Call(ctx context.Context, tool string, arguments json.RawMessage) (Result, error) {
	return callTool(ctx, u.server.Name, u.call, tool, arguments)
}

// Close ends the session.
func (u *Upstream) Close() {
	u.mu.Lock()
	session := u.session
	u.mu.Unlock()
	if session != "" {
		request, err := http.NewRequest(http.MethodDelete, u.server.URL, nil)
		if err == nil {
			u.decorate(request, session)
			if answer, err := u.client.Do(request); err == nil {
				answer.Body.Close()
			}
		}
	}
	u.client.CloseIdleConnections()
}

var errSessionGone = errors.New("the server's session is gone")

func (u *Upstream) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ctx, stop := context.WithTimeout(ctx, callTime)
	defer stop()
	for attempt := 0; ; attempt++ {
		if err := u.start(ctx); err != nil {
			return nil, err
		}
		raw, err := u.exchange(ctx, method, params)
		if errors.Is(err, errSessionGone) && attempt == 0 {
			u.mu.Lock()
			u.ready, u.session = false, ""
			u.mu.Unlock()
			continue
		}
		return raw, err
	}
}

func (u *Upstream) start(ctx context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.ready {
		return nil
	}
	init := map[string]any{"protocolVersion": protocols[0], "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "gusto-gateway", "version": "1"}}
	answer, session, err := u.post(ctx, "", 1, "initialize", init)
	if err != nil {
		return fmt.Errorf("%s did not start a session: %w", u.server.Name, err)
	}
	_ = answer
	u.session, u.ready = session, true
	if _, _, err := u.post(ctx, session, 0, "notifications/initialized", nil); err != nil {
		u.ready = false
		return fmt.Errorf("%s did not take the start of the session: %w", u.server.Name, err)
	}
	return nil
}

func (u *Upstream) exchange(ctx context.Context, method string, params any) (json.RawMessage, error) {
	u.mu.Lock()
	session := u.session
	u.mu.Unlock()
	raw, _, err := u.post(ctx, session, u.ids.Add(1)+1, method, params)
	return raw, err
}

func (u *Upstream) decorate(request *http.Request, session string) {
	for name, value := range u.server.Headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("MCP-Protocol-Version", protocols[0])
	if session != "" {
		request.Header.Set("Mcp-Session-Id", session)
	}
}

// post sends one message. A request (id above zero) is answered by its result, taken from a JSON body or from the event stream the
// server answers with, and a notification by nothing.
func (u *Upstream) post(ctx context.Context, session string, id int64, method string, params any) (json.RawMessage, string, error) {
	message := map[string]any{"jsonrpc": "2.0", "method": method}
	if id > 0 {
		message["id"] = id
	}
	if params != nil {
		message["params"] = params
	}
	body, err := json.Marshal(message)
	if err != nil {
		return nil, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.server.URL, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	u.decorate(request, session)
	answer, err := u.client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer answer.Body.Close()
	newSession := answer.Header.Get("Mcp-Session-Id")
	switch {
	case answer.StatusCode == http.StatusNotFound && session != "":
		return nil, "", errSessionGone
	case answer.StatusCode >= 300 && answer.StatusCode < 400:
		return nil, "", errors.New("the server answered with a redirect, which is not followed")
	case answer.StatusCode >= 400:
		detail, _ := io.ReadAll(io.LimitReader(answer.Body, 512))
		return nil, "", fmt.Errorf("the server answered %d %s", answer.StatusCode, strings.TrimSpace(string(detail)))
	case id == 0:
		io.Copy(io.Discard, io.LimitReader(answer.Body, 1<<16))
		return nil, newSession, nil
	}
	raw, err := resultOf(answer, id)
	return raw, newSession, err
}

// resultOf reads the answer to one request from a JSON body, or from the events of a stream, which may carry other messages first.
func resultOf(answer *http.Response, id int64) (json.RawMessage, error) {
	limited := io.LimitReader(answer.Body, mostAnswer+1)
	if strings.HasPrefix(answer.Header.Get("Content-Type"), "text/event-stream") {
		return fromStream(limited, id)
	}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > mostAnswer {
		return nil, errors.New("the answer is larger than the gateway reads")
	}
	return fromMessage(data, id)
}

func fromStream(reader io.Reader, id int64) (json.RawMessage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64<<10), mostAnswer)
	var data []string
	flush := func() (json.RawMessage, bool, error) {
		if len(data) == 0 {
			return nil, false, nil
		}
		joined := strings.Join(data, "\n")
		data = nil
		raw, err := fromMessage([]byte(joined), id)
		if errors.Is(err, errOtherMessage) {
			return nil, false, nil
		}
		return raw, true, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if raw, done, err := flush(); done {
				return raw, err
			}
			continue
		}
		if value, found := strings.CutPrefix(line, "data:"); found {
			data = append(data, strings.TrimPrefix(value, " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if raw, done, err := flush(); done {
		return raw, err
	}
	return nil, errors.New("the stream ended without an answer")
}

var errOtherMessage = errors.New("a message that is not the answer")

func fromMessage(data []byte, id int64) (json.RawMessage, error) {
	var reply struct {
		ID     json.RawMessage `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return nil, fmt.Errorf("the answer is not JSON: %w", err)
	}
	if reply.Method != "" || string(bytes.TrimSpace(reply.ID)) != fmt.Sprint(id) {
		return nil, errOtherMessage
	}
	if reply.Error != nil {
		return nil, reply.Error
	}
	return reply.Result, nil
}
