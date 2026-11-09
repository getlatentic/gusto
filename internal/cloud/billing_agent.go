package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// An organization's agents reach billing through one MCP endpoint (streamable HTTP, JSON-RPC 2.0) with two tools.
// One reads the state. The other proposes a change of seats, which an admin answers on the organization page and
// the payer approves at the processor. No tool buys or changes anything by itself.

const (
	mcpDefaultVersion = "2025-03-26"
	mcpBodyMax        = 16 << 10
	reasonMax         = 300
)

type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Name            string          `json:"name"`
		Arguments       json.RawMessage `json:"arguments"`
	} `json:"params"`
}

type rpcReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpText struct {
	Content []map[string]string `json:"content"`
	IsError bool                `json:"isError,omitempty"`
}

func text(message string, isError bool) mcpText {
	return mcpText{Content: []map[string]string{{"type": "text", "text": message}}, IsError: isError}
}

var billingTools = []map[string]any{
	{"name": "billing_status", "description": "Say how this organization pays for gusto cloud: its state (pending, active, past_due, suspended or not_subscribed), the processor it pays through, the seats it pays for, how many people it has, the price of a seat, whether its seats follow its people, and any seat change waiting for an answer.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}},
	{"name": "billing_propose_seats", "description": "Propose a new number of paid seats for this organization, with the reason. This only records a proposal. An admin must approve it on the organization page and the payer must approve it at the payment processor before anything changes or is charged. Ask a person before proposing.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"seats", "reason"}, "additionalProperties": false, "properties": map[string]any{
			"seats":  map[string]any{"type": "integer", "minimum": 1, "maximum": MostSeats},
			"reason": map[string]any{"type": "string", "maxLength": reasonMax}}}},
}

// billingMCP answers an organization's agent. The bearer token names the organization, and nothing in a request does.
func (s *Server) billingMCP(w http.ResponseWriter, r *http.Request) {
	org, ok := s.agentOrg(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="gusto billing"`)
		http.Error(w, "a billing token is needed", http.StatusUnauthorized)
		return
	}
	if !s.billingCalls.Allow(r.Context(), org) {
		http.Error(w, "too many calls", http.StatusTooManyRequests)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mcpBodyMax))
	var call rpcRequest
	if err != nil || json.Unmarshal(body, &call) != nil {
		writeRPC(w, rpcReply{Error: &rpcError{Code: -32700, Message: "not JSON-RPC"}})
		return
	}
	if len(call.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRPC(w, s.answerRPC(r.Context(), org, call))
}

func (s *Server) agentOrg(r *http.Request) (string, bool) {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || token == "" || len(token) > longestSecret {
		return "", false
	}
	org, found, err := s.Store.OrgOfAgentToken(token)
	return org, found && err == nil
}

func writeRPC(w http.ResponseWriter, reply rpcReply) {
	reply.JSONRPC = "2.0"
	if len(reply.ID) == 0 {
		reply.ID = json.RawMessage("null")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

func (s *Server) answerRPC(ctx context.Context, org string, call rpcRequest) rpcReply {
	reply := rpcReply{ID: call.ID}
	switch call.Method {
	case "initialize":
		version := call.Params.ProtocolVersion
		if version == "" {
			version = mcpDefaultVersion
		}
		reply.Result = map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]string{"name": "gusto-billing", "version": "1"}}
	case "ping":
		reply.Result = map[string]any{}
	case "tools/list":
		reply.Result = map[string]any{"tools": billingTools}
	case "tools/call":
		reply.Result = s.callTool(ctx, org, call.Params.Name, call.Params.Arguments)
	default:
		reply.Error = &rpcError{Code: -32601, Message: "no such method"}
	}
	return reply
}

func (s *Server) callTool(ctx context.Context, org, name string, arguments json.RawMessage) mcpText {
	switch name {
	case "billing_status":
		return s.toolStatus(ctx, org)
	case "billing_propose_seats":
		return s.toolPropose(org, arguments)
	}
	return text("no such tool: "+name, true)
}

func (s *Server) toolStatus(ctx context.Context, org string) mcpText {
	status := map[string]any{"state": "not_subscribed"}
	people, err := s.Store.Members(ctx, org)
	if err != nil {
		return text("billing could not be read", true)
	}
	status["people"] = len(people)
	plan, found, err := s.Store.Plan(ctx, org)
	if err != nil {
		return text("billing could not be read", true)
	}
	if found {
		status["state"], status["seats_paid"], status["seats_follow_people"], status["provider"] = string(plan.State), plan.Seats, !plan.Fixed, plan.Provider
		if p, ok := s.Billing.Providers.Get(plan.Provider); ok {
			status["price_per_seat"] = p.Price()
		}
	}
	waiting, _ := s.Store.OpenProposals(org)
	status["seat_changes_waiting"] = len(waiting)
	for _, p := range waiting {
		status["waiting_seats"], status["waiting_status"] = p.Seats, string(p.Status)
	}
	out, _ := json.Marshal(status)
	return text(string(out), false)
}

func (s *Server) toolPropose(org string, arguments json.RawMessage) mcpText {
	var in struct {
		Seats  int    `json:"seats"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(arguments, &in); err != nil {
		return text("seats must be a whole number and reason a sentence", true)
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || len(reason) > reasonMax {
		return text("give a reason of up to 300 characters", true)
	}
	proposal, err := s.Store.Propose(org, in.Seats, reason, "agent")
	for _, known := range billingSentences {
		if errors.Is(err, known) {
			return text(known.Error(), true)
		}
	}
	if err != nil {
		return text("the proposal could not be recorded", true)
	}
	return text("Proposed "+strconv.Itoa(proposal.Seats)+" seats. It is waiting for an admin to approve it on the organization page, and for the payer to approve it at the payment processor. Nothing has changed and nothing has been charged.", false)
}
