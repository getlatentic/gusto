package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// upstream is an MCP server of the kind the gateway calls: it keeps a session, lists tools in pages, calls them, and answers as
// JSON or as an event stream.
type upstream struct {
	t       *testing.T
	mu      sync.Mutex
	tools   []Tool
	token   string
	stream  bool
	calls   []string
	headers []http.Header
	gone    bool
	session int
	inits   int
}

func tool(name, description string) Tool {
	return Tool{Name: name, Description: description, InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"` + strings.Repeat("an identifier ", 20) + `"}}}`)}
}

func newUpstream(t *testing.T, tools ...Tool) (*upstream, string) {
	t.Helper()
	fake := &upstream{t: t, tools: tools, token: "s3cret"}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server.URL
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.headers = append(u.headers, r.Header.Clone())
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusOK)
		return
	}
	if u.token != "" && r.Header.Get("Authorization") != "Bearer "+u.token {
		http.Error(w, "no", http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var in request
	json.Unmarshal(body, &in)
	if in.Method != "initialize" && (u.gone || r.Header.Get("Mcp-Session-Id") != fmt.Sprint("session-", u.session)) {
		u.gone = false
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if in.notification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var result any
	switch in.Method {
	case "initialize":
		u.inits++
		u.session++
		w.Header().Set("Mcp-Session-Id", fmt.Sprint("session-", u.session))
		result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "serverInfo": map[string]string{"name": "fake"}}
	case "tools/list":
		var page struct{ Cursor string }
		json.Unmarshal(in.Params, &page)
		start := 0
		fmt.Sscan(page.Cursor, &start)
		end := min(start+3, len(u.tools))
		listing := map[string]any{"tools": u.tools[start:end]}
		if end < len(u.tools) {
			listing["nextCursor"] = fmt.Sprint(end)
		}
		result = listing
	case "tools/call":
		var asked struct {
			Name      string
			Arguments json.RawMessage
		}
		json.Unmarshal(in.Params, &asked)
		u.calls = append(u.calls, asked.Name+" "+string(asked.Arguments))
		if asked.Name == "explode" {
			result = text("it failed", true)
		} else {
			result = text("did "+asked.Name+" with "+string(asked.Arguments), false)
		}
	default:
		http.Error(w, "unknown", http.StatusBadRequest)
		return
	}
	reply, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(in.ID), "result": result})
	if u.stream {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/message\",\"params\":{}}\n\ndata: %s\n\n", reply)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(reply)
}

// gateway is a job's gateway over servers, and a way to talk to it as a harness does.
type harness struct {
	t       *testing.T
	session *Session
	server  *httptest.Server
	record  string
	next    atomic.Int64
}

func start(t *testing.T, config Config) *harness {
	t.Helper()
	config.Private = true
	if config.Record == nil {
		path := t.TempDir() + "/" + RecordFile
		config.Record = OpenRecord(path)
	}
	if config.Caller.Job == "" {
		config.Caller = Caller{Job: "20261008-1", Person: "ada@acme.example", Team: "payments"}
	}
	session := New(config)
	t.Cleanup(session.Close)
	server := httptest.NewServer(session)
	t.Cleanup(server.Close)
	return &harness{t: t, session: session, server: server, record: config.Record.path}
}

func (h *harness) send(method string, params any) response {
	h.t.Helper()
	message, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": h.next.Add(1), "method": method, "params": params})
	answer, err := http.Post(h.server.URL, "application/json", strings.NewReader(string(message)))
	if err != nil {
		h.t.Fatal(err)
	}
	defer answer.Body.Close()
	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.NewDecoder(answer.Body).Decode(&reply); err != nil {
		h.t.Fatalf("the gateway's answer to %s is not JSON: %v", method, err)
	}
	var value any
	json.Unmarshal(reply.Result, &value)
	return response{Result: value, Error: reply.Error}
}

// call is a tools/call and the first text it answers with, and whether it said it failed.
func (h *harness) call(name string, arguments any) (string, bool) {
	h.t.Helper()
	reply := h.send("tools/call", map[string]any{"name": name, "arguments": arguments})
	if reply.Error != nil {
		h.t.Fatalf("%s: %v", name, reply.Error)
	}
	result := reply.Result.(map[string]any)
	parts := result["content"].([]any)
	failed, _ := result["isError"].(bool)
	return parts[0].(map[string]any)["text"].(string), failed
}

func (h *harness) entries() []Entry {
	h.t.Helper()
	data, _ := os.ReadFile(h.record)
	var out []Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, entry)
	}
	return out
}
