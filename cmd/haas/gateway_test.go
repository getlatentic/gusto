package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/setup"
	"github.com/getlatentic/haas/internal/spend"
)

// toolServer is an MCP server that has one tool, and keeps what it was called with and by what credential.
func toolServer(t *testing.T) (address string, calls *[]string) {
	t.Helper()
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		json.Unmarshal(body, &in)
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if len(in.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch in.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "s1")
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "lookup", "description": "Look a customer up by name.", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			seen = append(seen, string(in.Params))
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": "found them"}}}
		}
		reply, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": in.ID, "result": result})
		w.Write(reply)
	}))
	t.Cleanup(server.Close)
	return server.URL, &seen
}

// proxied is a proxy that serves the gateways of the jobs under a folder, with one job that has a server behind the gateway.
func proxied(t *testing.T, upstream string) (front *httptest.Server, token, root string) {
	t.Helper()
	t.Setenv("HAAS_GATEWAY_PRIVATE", "1")
	root = t.TempDir()
	dir := job.Dir(filepath.Join(root, "20261008-1200-a-turn"))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "x", Repo: "r", Harness: "fixture", Team: "payments", Sensitivity: job.Internal,
		Budget: job.Budget{Turns: 1, Seconds: 60}, Setup: &job.Setup{Name: "team", Gateway: []string{"crm"}}}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir.Path(), "state.json"), []byte(`{"id":"20261008-1200-a-turn","asked":"ada@acme.example"}`), 0o644)
	os.MkdirAll(dir.SetupDir(), 0o700)
	definitions, _ := json.Marshal([]map[string]any{
		{"name": "crm", "url": upstream, "headers": map[string]string{"Authorization": "Bearer upstream-secret"}},
		{"name": "other", "url": "http://nowhere.invalid"},
	})
	os.WriteFile(filepath.Join(dir.SetupDir(), setup.GatewayFile), definitions, 0o600)

	gates, err := openGateways(root)
	if err != nil {
		t.Fatal(err)
	}
	tokens := proxy.NewTokens()
	where := func(id string) (string, bool) { return filepath.Join(root, id, "out", "proxy.jsonl"), true }
	p := proxy.New(tokens, proxy.Credentials{}, where, spend.NewLedger(spend.Policy{}), proxy.ParseAllowlist(""))
	p.UseGateways(gates.For)
	front = httptest.NewServer(p.Handler())
	t.Cleanup(front.Close)
	token, err = tokens.Mint("20261008-1200-a-turn", "")
	if err != nil {
		t.Fatal(err)
	}
	return front, token, root
}

func rpc(t *testing.T, address, token, method string, params any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	request, _ := http.NewRequest("POST", address, strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer answer.Body.Close()
	var reply map[string]any
	if err := json.NewDecoder(answer.Body).Decode(&reply); err != nil {
		return map[string]any{"status": float64(answer.StatusCode)}
	}
	reply["status"] = float64(answer.StatusCode)
	return reply
}

func TestAJobReachesItsToolServersThroughTheProxyAndNothingElse(t *testing.T) {
	upstream, calls := toolServer(t)
	front, token, root := proxied(t, upstream)
	endpoint := front.URL + "/model/mcp"

	listed := rpc(t, endpoint, token, "tools/list", nil)
	tools := listed["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("the job is shown %d tools, want the gateway's three: %v", len(tools), listed)
	}
	found := rpc(t, endpoint, token, "tools/call", map[string]any{"name": "tool_search", "arguments": map[string]any{"query": "customer"}})
	if text := fmt.Sprint(found["result"]); !strings.Contains(text, "crm/lookup") || strings.Contains(text, "other") {
		t.Errorf("search = %s", text)
	}
	rpc(t, endpoint, token, "tools/call", map[string]any{"name": "tool_call", "arguments": map[string]any{"name": "crm/lookup", "arguments": map[string]any{"name": "Acme"}}})
	if len(*calls) != 1 || !strings.Contains((*calls)[0], "Acme") {
		t.Fatalf("the server was called %v", *calls)
	}

	record, _ := os.ReadFile(filepath.Join(root, "20261008-1200-a-turn", "out", "gateway.jsonl"))
	if !strings.Contains(string(record), `"person":"ada@acme.example"`) || !strings.Contains(string(record), `"tool":"lookup"`) || strings.Contains(string(record), "upstream-secret") {
		t.Errorf("the record = %s", record)
	}

	if got := rpc(t, endpoint, "", "tools/list", nil)["status"]; got != float64(403) {
		t.Errorf("a call with no token answered %v", got)
	}
	if got := rpc(t, endpoint, "not-a-token", "tools/list", nil)["status"]; got != float64(403) {
		t.Errorf("a call with another token answered %v", got)
	}
}

func TestAJobWithNothingBehindTheGatewayHasNoGateway(t *testing.T) {
	upstream, _ := toolServer(t)
	front, _, root := proxied(t, upstream)
	plain := job.Dir(filepath.Join(root, "20261008-1300-plain"))
	plain.Make()
	job.Spec{Brief: "x", Repo: "r", Harness: "fixture", Sensitivity: job.Internal, Budget: job.Budget{Turns: 1, Seconds: 60}}.Write(plain)
	gates, _ := openGateways(root)
	if _, has := gates.For("20261008-1300-plain"); has {
		t.Error("a job with no servers behind the gateway has one")
	}
	if _, has := gates.For("../x"); has {
		t.Error("a job id that climbs was taken")
	}
	_ = front
}
