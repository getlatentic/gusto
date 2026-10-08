//go:build realclients

package gateway

// This runs the programs people really use as MCP clients against the gateway, which no in-process test can say is right:
//
//	go test -tags realclients -run RealClient ./internal/gateway
//
// It needs claude and node on the PATH, and npx to fetch the protocol's own client library.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealClientClaudeConnectsAndLists(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("no claude")
	}
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	dir := t.TempDir()
	run := func(args ...string) string {
		command := exec.Command("claude", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "HOME="+dir)
		done := make(chan struct{})
		var out []byte
		var err error
		go func() { out, err = command.CombinedOutput(); close(done) }()
		select {
		case <-done:
		case <-time.After(90 * time.Second):
			command.Process.Kill()
			t.Fatalf("claude %v took too long", args)
		}
		if err != nil {
			t.Logf("claude %v: %v", args, err)
		}
		return string(out)
	}
	run("mcp", "add", "--transport", "http", "--scope", "user", "gateway", h.server.URL, "--header", "Authorization: Bearer job-token")
	listed := run("mcp", "list")
	if !strings.Contains(listed, "gateway") || !strings.Contains(listed, "Connected") {
		t.Fatalf("claude does not connect to the gateway:\n%s", listed)
	}
}

// The protocol's own client library lists the three tools, reads a schema, and calls a tool through them.
func TestRealClientTheProtocolsOwnSDK(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("no npm")
	}
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	dir := t.TempDir()
	script := `
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";
const client = new Client({ name: "check", version: "1" });
await client.connect(new StreamableHTTPClientTransport(new URL(process.env.GATEWAY)));
const listed = await client.listTools();
console.log("tools:" + listed.tools.map((t) => t.name).join(","));
const found = await client.callTool({ name: "tool_search", arguments: { query: "report" } });
console.log("found:" + found.content[0].text.split("\n").map((l) => l.split(":")[0]).join(","));
const said = await client.callTool({ name: "tool_call", arguments: { name: "reports/get_report", arguments: { id: "q3" } } });
console.log("said:" + said.content[0].text);
await client.close();
`
	os.WriteFile(filepath.Join(dir, "check.mjs"), []byte(script), 0o644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"type":"module"}`), 0o644)
	install := exec.Command("npm", "install", "--silent", "@modelcontextprotocol/sdk")
	install.Dir = dir
	if out, err := install.CombinedOutput(); err != nil {
		t.Skipf("the SDK could not be installed: %v\n%s", err, out)
	}
	command := exec.Command("node", "check.mjs")
	command.Dir = dir
	command.Env = append(os.Environ(), "GATEWAY="+h.server.URL)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("the SDK's client: %v\n%s", err, out)
	}
	for _, want := range []string{"tools:tool_search,tool_schema,tool_call", "reports/get_report", `said:did get_report with {"id":"q3"}`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("the SDK's client did not say %q:\n%s", want, out)
		}
	}
}
