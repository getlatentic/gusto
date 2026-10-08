package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getlatentic/haas/internal/redact"
)

// Mode is how a job is shown its tools.
type Mode string

const (
	// Discover shows three tools, to search the others, read one's schema and call one, so the definitions of the tools a job never
	// uses are never in its context.
	Discover Mode = "discover"
	// All shows every tool the caller may use, each with its definition, the way a harness is given an MCP server directly.
	All Mode = "all"
)

const (
	mostRequest = 1 << 20
	listed      = 8
	mostListed  = 25
)

// Config is what a job's gateway is made from.
type Config struct {
	Caller  Caller
	Servers []Server
	Policy  Policy
	// Live, when set, is the policy as it is now, asked for at each call, and Policy is not used.
	Live func() Policy
	// Redactor, when set, is what masks personal data and secrets in what a tool answers, asked for at each call.
	Redactor func() *redact.Redactor
	Mode     Mode
	// Private lets the servers be on a private network.
	Private bool
	// Launch makes the command that runs a server that is a program. Without it a program is not run.
	Launch Launcher
	Record *Record
}

// Session is the gateway for one job: an MCP server the job's harness calls.
type Session struct {
	config    Config
	catalogue *Catalogue
	now       func() time.Time
	// recordFailed is the error that stopped a line being written, which stops the calls after it.
	recordFailed atomic.Pointer[error]
}

// New is the gateway for a job.
func New(config Config) *Session {
	if config.Mode == "" {
		config.Mode = Discover
	}
	policy := config.Live
	if policy == nil {
		policy = func() Policy { return config.Policy }
	}
	return &Session{config: config, now: time.Now, catalogue: newCatalogue(config.Servers, config.Private, config.Launch, policy, config.Caller, config.Record)}
}

// Close ends the sessions with the servers.
func (s *Session) Close() { s.catalogue.close() }

// ServeHTTP answers one message, or a batch. It keeps no session of its own, and offers no stream: what a server would say unasked,
// the job's tools do not.
func (s *Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Header.Get("Origin") != "":
		http.Error(w, "a page in a browser may not call this", http.StatusForbidden)
		return
	case r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method != http.MethodPost:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "this takes messages by POST", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, mostRequest+1))
	if err != nil || len(body) > mostRequest {
		answer(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeInvalid, "the message is too large"}})
		return
	}
	body = bytes.TrimSpace(body)
	if len(body) > 0 && body[0] == '[' {
		var batch []request
		if json.Unmarshal(body, &batch) != nil {
			answer(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "not JSON"}})
			return
		}
		var replies []response
		for _, one := range batch {
			if reply, wanted := s.handle(r, one); wanted {
				replies = append(replies, reply)
			}
		}
		if len(replies) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		answer(w, replies)
		return
	}
	var one request
	if err := json.Unmarshal(body, &one); err != nil || one.Method == "" {
		answer(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "not a JSON-RPC message"}})
		return
	}
	reply, wanted := s.handle(r, one)
	if !wanted {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	answer(w, reply)
}

func answer(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

func (s *Session) handle(r *http.Request, in request) (response, bool) {
	if in.notification() {
		return response{}, false
	}
	reply := response{JSONRPC: "2.0", ID: in.ID}
	switch in.Method {
	case "initialize":
		var asked struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(in.Params, &asked)
		reply.Result = map[string]any{
			"protocolVersion": agreed(asked.ProtocolVersion),
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]string{"name": "haas-gateway", "version": "1"},
			"instructions":    s.instructions(),
		}
	case "ping":
		reply.Result = map[string]any{}
	case "tools/list":
		reply.Result = map[string]any{"tools": s.list(r)}
	case "tools/call":
		var asked struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(in.Params, &asked) != nil || asked.Name == "" {
			reply.Error = &rpcError{codeParams, "a call names a tool"}
			break
		}
		reply.Result = s.call(r, asked.Name, asked.Arguments)
	default:
		reply.Error = &rpcError{codeMethod, "this server does not know " + in.Method}
	}
	return reply, true
}

func (s *Session) instructions() string {
	if s.config.Mode == All {
		return "These are the tools of the servers this job was given."
	}
	return "Tools are found as they are needed. Use tool_search to find one, tool_schema to read how to call it, and tool_call to call it."
}

// list is the tools the job is shown, and writes down how many there are and what showing them all would cost.
func (s *Session) list(r *http.Request) []Tool {
	s.catalogue.load(r.Context())
	shown := s.shownTools()
	if s.config.Mode == All {
		shown = shown[:0]
		for _, candidate := range s.catalogue.visible() {
			tool := candidate.Tool
			tool.Name = candidate.Flat
			shown = append(shown, tool)
		}
	}
	s.note(Entry{Kind: "list", Tools: len(s.catalogue.visible()), TokensAll: definitionsTokens(s.catalogue.tools()), TokensShown: definitionsTokens(shown)})
	return shown
}

func (s *Session) shownTools() []Tool {
	object := func(properties string, required ...string) json.RawMessage {
		if required == nil {
			required = []string{}
		}
		need, _ := json.Marshal(required)
		return json.RawMessage(`{"type":"object","properties":{` + properties + `},"required":` + string(need) + `}`)
	}
	return []Tool{
		{Name: "tool_search", Description: "Find tools by what they do. Returns names to use with tool_schema and tool_call.",
			InputSchema: object(`"query":{"type":"string"},"server":{"type":"string"},"limit":{"type":"integer"}`)},
		{Name: "tool_schema", Description: "How to call a tool: its description and the arguments it takes.",
			InputSchema: object(`"name":{"type":"string"}`, "name")},
		{Name: "tool_call", Description: "Call a tool by its name from tool_search, with its arguments.",
			InputSchema: object(`"name":{"type":"string"},"arguments":{"type":"object"}`, "name")},
	}
}

func (s *Session) note(entry Entry) {
	entry.Job, entry.Person = s.config.Caller.Job, s.config.Caller.Person
	if err := s.config.Record.Add(entry); err != nil {
		// A record that cannot be written is not hidden: the next call is refused for it.
		s.recordFailed.Store(&err)
	}
}

func (s *Session) call(r *http.Request, name string, arguments json.RawMessage) Result {
	s.catalogue.load(r.Context())
	if failed := s.recordFailed.Load(); failed != nil {
		return text("the gateway cannot write down this job's calls, so it makes none: "+(*failed).Error(), true)
	}
	switch name {
	case "tool_search":
		return s.search(arguments)
	case "tool_schema":
		return s.schema(arguments)
	case "tool_call":
		var asked struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(arguments, &asked) != nil || asked.Name == "" {
			return text("tool_call needs the name of a tool", true)
		}
		return s.invoke(r, asked.Name, asked.Arguments)
	}
	if s.config.Mode == All {
		return s.invoke(r, name, arguments)
	}
	return text(fmt.Sprintf("there is no tool %q here: use tool_search to find one, and tool_call to call it", name), true)
}

func (s *Session) search(arguments json.RawMessage) Result {
	var asked struct {
		Query  string `json:"query"`
		Server string `json:"server"`
		Limit  int    `json:"limit"`
	}
	_ = json.Unmarshal(arguments, &asked)
	limit := min(max(asked.Limit, 0), mostListed)
	if limit == 0 {
		limit = listed
	}
	found := s.catalogue.search(asked.Query, asked.Server, limit)
	var out strings.Builder
	for _, hit := range found {
		fmt.Fprintf(&out, "%s: %s\n", hit.Name, oneLine(hit.Tool.Description))
	}
	if out.Len() == 0 {
		out.WriteString("no tool matches")
	}
	for _, name := range s.catalogue.order {
		if why, down := s.catalogue.broken[name]; down {
			fmt.Fprintf(&out, "(the server %s could not be reached: %s)\n", name, oneLine(why))
		}
	}
	s.note(Entry{Kind: "search", Outcome: "ok", Tools: len(found)})
	return text(strings.TrimSpace(out.String()), false)
}

func (s *Session) schema(arguments json.RawMessage) Result {
	var asked struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(arguments, &asked) != nil || asked.Name == "" {
		return text("tool_schema needs the name of a tool", true)
	}
	found, ok := s.catalogue.find(asked.Name)
	if !ok {
		return text(fmt.Sprintf("there is no tool %q: use tool_search to find one", asked.Name), true)
	}
	definition, _ := json.Marshal(map[string]any{"name": found.Name, "description": found.Tool.Description, "inputSchema": found.Tool.InputSchema})
	s.note(Entry{Kind: "schema", Server: found.Server, Tool: found.Tool.Name, Outcome: "ok"})
	return text(string(definition), false)
}

func (s *Session) invoke(r *http.Request, name string, arguments json.RawMessage) Result {
	entry := Entry{Kind: "call", Tool: name, ArgBytes: len(arguments)}
	found, ok := s.catalogue.find(name)
	if ok {
		if allowed, why := s.catalogue.allows(found); !allowed {
			entry.Server, entry.Tool, entry.Outcome, entry.Why = found.Server, found.Tool.Name, "refused", why
			s.note(entry)
			return text(why, true)
		}
	}
	if !ok {
		entry.Outcome, entry.Why = "refused", "no such tool for this caller"
		s.note(entry)
		return text(fmt.Sprintf("there is no tool %q that you may use: use tool_search to find one", name), true)
	}
	entry.Server, entry.Tool = found.Server, found.Tool.Name
	began := s.now()
	result, err := s.catalogue.servers[found.Server].Call(r.Context(), found.Tool.Name, arguments)
	entry.Millis = s.now().Sub(began).Milliseconds()
	if err != nil {
		entry.Outcome, entry.Why = "failed", err.Error()
		s.note(entry)
		return text("the call could not be made: "+err.Error(), true)
	}
	result, entry.Redacted = s.masked(result)
	size, _ := json.Marshal(result)
	entry.ResultBytes, entry.Outcome = len(size), "ok"
	if result.IsError {
		entry.Outcome = "error"
	}
	s.note(entry)
	return result
}

// masked is what a tool answered with the personal data and secrets in it masked, and how many of each. The answer is masked as the
// message it is, so that it is the same shape, and what was masked is not kept.
func (s *Session) masked(result Result) (Result, map[string]int) {
	if s.config.Redactor == nil {
		return result, nil
	}
	redactor := s.config.Redactor()
	if !redactor.On() {
		return result, nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return result, nil
	}
	masked, counts := redactor.JSON(encoded)
	if len(counts) == 0 {
		return result, nil
	}
	var again Result
	if json.Unmarshal(masked, &again) != nil {
		return text("the answer held something that had to be masked, and it could not be returned masked", true), counts
	}
	return again, counts
}

func oneLine(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	for _, end := range []string{". ", "! ", "? "} {
		if cut := strings.Index(line, end); cut > 0 && cut < 200 {
			line = line[:cut+1]
		}
	}
	if len([]rune(line)) > 200 {
		line = string([]rune(line)[:199]) + "…"
	}
	return line
}
