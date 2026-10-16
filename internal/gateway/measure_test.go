//go:build realclients

package gateway

// This measures what the gateway saves a real model, with the real claude program:
//
//	GUSTO_MEASURE=1 go test -tags realclients -run Measure -v ./internal/gateway
//
// It makes three short calls to the model claude is signed in to, so it only runs when asked for.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// realisticTools are tools with descriptions and schemas of the size real servers have.
func realisticTools(n int) []Tool {
	var tools []Tool
	for i := range n {
		schema := fmt.Sprintf(`{"type":"object","properties":{"id":{"type":"string","description":"The identifier of the record to work on. Use the id returned by the list tool."},"fields":{"type":"array","items":{"type":"string"},"description":"Which fields of the record to return. Leave out for all of them."},"include_archived":{"type":"boolean","description":"Whether to include archived records in the result, which are left out by default."},"limit":{"type":"integer","description":"The most records to return, between 1 and 100."}},"required":["id"],"additionalProperties":false}`)
		tools = append(tools, Tool{
			Name:        fmt.Sprintf("operation_%02d", i),
			Description: "Looks up, changes or lists one kind of record held by the customer system, and says what it found or did. Use it when the person asks about customers, orders, invoices or support cases, and not for anything that is only about the code in the repository.",
			InputSchema: json.RawMessage(schema),
		})
	}
	return tools
}

func tokensOf(t *testing.T, config string) (input, output int, said string) {
	t.Helper()
	dir := t.TempDir()
	args := []string{"-p", "Reply with the single word ok.", "--output-format", "json", "--max-turns", "1", "--strict-mcp-config", "--model", "haiku"}
	if config != "" {
		path := filepath.Join(dir, "mcp.json")
		os.WriteFile(path, []byte(config), 0o644)
		args = append(args, "--mcp-config", path)
	}
	command := exec.Command("claude", args...)
	command.Dir = dir
	command.Env = os.Environ()
	done := make(chan struct{})
	var out []byte
	var err error
	go func() { out, err = command.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		command.Process.Kill()
		t.Fatal("claude took too long")
	}
	if err != nil {
		t.Fatalf("claude: %v\n%s", err, out)
	}
	var result struct {
		Result string `json:"result"`
		Usage  struct {
			Input         int `json:"input_tokens"`
			CacheCreation int `json:"cache_creation_input_tokens"`
			CacheRead     int `json:"cache_read_input_tokens"`
			Output        int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	u := result.Usage
	return u.Input + u.CacheCreation + u.CacheRead, u.Output, strings.TrimSpace(result.Result)
}

func TestMeasureWhatTheGatewaySavesARealModel(t *testing.T) {
	if os.Getenv("GUSTO_MEASURE") == "" {
		t.Skip("set GUSTO_MEASURE=1 to make three short calls to claude's model")
	}
	fake, address := newUpstream(t, realisticTools(60)...)
	_ = fake
	server := Server{Name: "customers", URL: address, Headers: map[string]string{"Authorization": "Bearer s3cret"}}
	config := func(h *harness) string {
		return `{"mcpServers":{"gateway":{"type":"http","url":"` + h.server.URL + `"}}}`
	}
	all := start(t, Config{Servers: []Server{server}, Mode: All})
	discover := start(t, Config{Servers: []Server{server}, Mode: Discover})
	_, tinyAddress := newUpstream(t, realisticTools(1)...)
	tiny := start(t, Config{Servers: []Server{{Name: "tiny", URL: tinyAddress, Headers: map[string]string{"Authorization": "Bearer s3cret"}}}, Mode: All})
	configs := []struct{ name, config string }{
		{"no tools", ""},
		{"one tool", config(tiny)},
		{"all 60 tools", config(all)},
		{"the three tools", config(discover)},
	}
	// Each is asked for three times, in turn, because one call's count moves with what the provider had cached.
	counts := map[string][]int{}
	for round := 0; round < 3; round++ {
		for _, c := range configs {
			input, _, _ := tokensOf(t, c.config)
			counts[c.name] = append(counts[c.name], input)
		}
	}
	for _, c := range configs {
		t.Logf("%-16s input tokens in three calls: %v", c.name, counts[c.name])
	}
	var estimate Entry
	for _, entry := range all.entries() {
		if entry.Kind == "list" {
			estimate = entry
		}
	}
	t.Logf("the gateway's own estimate of showing all %d tools: %d tokens, against %d for three", estimate.Tools, estimate.TokensAll, estimate.TokensShown)
}

// codexInput is the input tokens codex's model read for one short turn, with the gateway's address as its one MCP server, or none.
func codexInput(t *testing.T, address string) int {
	t.Helper()
	dir := t.TempDir()
	args := []string{"exec", "--json", "--skip-git-repo-check", "--ephemeral", "-m", "gpt-5.5", "-C", dir}
	if address != "" {
		args = append(args, "-c", `mcp_servers.gateway.url="`+address+`"`)
	}
	args = append(args, "Reply with the single word ok.")
	command := exec.Command("codex", args...)
	command.Dir = dir
	command.Stdin = nil
	done := make(chan struct{})
	var out []byte
	var err error
	go func() { out, err = command.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(180 * time.Second):
		command.Process.Kill()
		t.Fatal("codex took too long")
	}
	input := 0
	for _, line := range strings.Split(string(out), "\n") {
		var event struct {
			Type  string `json:"type"`
			Usage struct {
				Input int `json:"input_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Type == "turn.completed" {
			input = event.Usage.Input
		}
	}
	if input == 0 {
		t.Fatalf("no usage from codex (%v):\n%s", err, out)
	}
	return input
}

func TestMeasureWhatTheGatewaySavesCodex(t *testing.T) {
	if os.Getenv("GUSTO_MEASURE") == "" {
		t.Skip("set GUSTO_MEASURE=1 to make short calls to codex's model")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("no codex")
	}
	_, address := newUpstream(t, realisticTools(60)...)
	server := Server{Name: "customers", URL: address, Headers: map[string]string{"Authorization": "Bearer s3cret"}}
	all := start(t, Config{Servers: []Server{server}, Mode: All})
	discover := start(t, Config{Servers: []Server{server}, Mode: Discover})
	counts := map[string][]int{}
	for round := 0; round < 3; round++ {
		counts["no tools"] = append(counts["no tools"], codexInput(t, ""))
		counts["all 60 tools"] = append(counts["all 60 tools"], codexInput(t, all.server.URL))
		counts["the three tools"] = append(counts["the three tools"], codexInput(t, discover.server.URL))
	}
	for name, h := range map[string]*harness{"all 60 tools": all, "the three tools": discover} {
		lists := 0
		for _, entry := range h.entries() {
			if entry.Kind == "list" {
				lists++
			}
		}
		t.Logf("codex asked the gateway for its tools %d times in the %s runs", lists, name)
	}
	for _, name := range []string{"no tools", "all 60 tools", "the three tools"} {
		t.Logf("codex, %-16s input tokens in three calls: %v", name, counts[name])
	}
}

// opencodeInput is the input tokens opencode's model read for one short turn, with the gateway's address as its one MCP server.
func opencodeInput(t *testing.T, address string) int {
	t.Helper()
	dir := t.TempDir()
	command := exec.Command("opencode", "run", "--format", "json", "-m", "opencode/big-pickle", "Reply with the single word ok.")
	command.Dir = dir
	command.Env = os.Environ()
	if address != "" {
		command.Env = append(command.Env, `OPENCODE_CONFIG_CONTENT={"mcp":{"gateway":{"type":"remote","url":"`+address+`","enabled":true}}}`)
	}
	done := make(chan struct{})
	var out []byte
	var err error
	go func() { out, err = command.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(180 * time.Second):
		command.Process.Kill()
		t.Fatal("opencode took too long")
	}
	input := 0
	for _, line := range strings.Split(string(out), "\n") {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Tokens struct {
					Input int `json:"input"`
					Cache struct {
						Write int `json:"write"`
						Read  int `json:"read"`
					} `json:"cache"`
				} `json:"tokens"`
			} `json:"part"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Type == "step_finish" {
			input = event.Part.Tokens.Input + event.Part.Tokens.Cache.Write + event.Part.Tokens.Cache.Read
		}
	}
	if input == 0 {
		t.Fatalf("no usage from opencode (%v):\n%s", err, out)
	}
	return input
}

func TestMeasureWhatTheGatewaySavesOpencode(t *testing.T) {
	if os.Getenv("GUSTO_MEASURE") == "" {
		t.Skip("set GUSTO_MEASURE=1 to make short calls to opencode's free model")
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("no opencode")
	}
	_, address := newUpstream(t, realisticTools(60)...)
	server := Server{Name: "customers", URL: address, Headers: map[string]string{"Authorization": "Bearer s3cret"}}
	all := start(t, Config{Servers: []Server{server}, Mode: All})
	discover := start(t, Config{Servers: []Server{server}, Mode: Discover})
	counts := map[string][]int{}
	for round := 0; round < 3; round++ {
		counts["no tools"] = append(counts["no tools"], opencodeInput(t, ""))
		counts["all 60 tools"] = append(counts["all 60 tools"], opencodeInput(t, all.server.URL))
		counts["the three tools"] = append(counts["the three tools"], opencodeInput(t, discover.server.URL))
	}
	for _, name := range []string{"no tools", "all 60 tools", "the three tools"} {
		t.Logf("opencode, %-16s input tokens in three calls: %v", name, counts[name])
	}
}
