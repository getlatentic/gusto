package harness

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// MCP is how a harness takes MCP servers. Each reads them in a format of its
// own, measured on 2026-09-27 against a probe server:
//
//	claude    --mcp-config with a JSON string, and --allowedTools for each
//	          server, since print mode denies a tool nobody allowed
//	codex     one -c mcp_servers={...} inline table, approved in advance,
//	          since exec never asks and refuses what needs asking
//	opencode  an "mcp" key in OPENCODE_CONFIG_CONTENT, merged with its file
//	acp       mcpServers on the session, which the runner opens
//
// A harness with no MCP has none of this, and a setup naming a server is
// refused for it before it starts.
type MCP struct {
	Format string `json:"format"`
	// Argv is added to the harness's arguments, and Env to its environment,
	// when a job names servers. {mcp} is the servers in Format, {mcpTools}
	// the names a harness allows its tools by.
	Argv []string          `json:"argv,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
}

// TakesMCP says whether this harness can be given MCP servers.
func (a Adapter) TakesMCP() bool { return a.MCP != nil }

// Give hands a built command the servers a job names, in this harness's own
// format. An ACP harness is given them on the session instead, by the runner.
func (a Adapter) Give(command *exec.Cmd, servers []job.Server) error {
	if len(servers) == 0 {
		return nil
	}
	if a.MCP == nil {
		return fmt.Errorf("%s cannot be given MCP servers", a.Name)
	}
	rendered, err := a.MCP.rendered(servers)
	if err != nil {
		return err
	}
	fill := strings.NewReplacer("{mcp}", rendered, "{mcpTools}", tools(servers))
	for _, argument := range a.MCP.Argv {
		command.Args = append(command.Args, fill.Replace(argument))
	}
	for key, value := range a.MCP.Env {
		command.Env = append(command.Env, key+"="+fill.Replace(value))
	}
	return nil
}

// rendered is the servers in this harness's format.
func (m MCP) rendered(servers []job.Server) (string, error) {
	switch m.Format {
	case "claude":
		return asJSON(map[string]any{"mcpServers": byName(servers, claudeServer)})
	case "opencode":
		return asJSON(map[string]any{"mcp": byName(servers, opencodeServer)})
	case "codex":
		return codexTable(servers), nil
	case "acp":
		return "", nil
	}
	return "", fmt.Errorf("no MCP format called %q", m.Format)
}

// tools is how claude names every tool of a server at once.
func tools(servers []job.Server) string {
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		names = append(names, "mcp__"+server.Name)
	}
	return strings.Join(names, ",")
}

func byName(servers []job.Server, shape func(job.Server) map[string]any) map[string]any {
	shaped := map[string]any{}
	for _, server := range servers {
		shaped[server.Name] = shape(server)
	}
	return shaped
}

func claudeServer(server job.Server) map[string]any {
	if server.URL != "" {
		return map[string]any{"type": "http", "url": server.URL, "headers": server.Headers}
	}
	return map[string]any{"command": server.Command, "args": orEmpty(server.Args), "env": server.Env}
}

func opencodeServer(server job.Server) map[string]any {
	if server.URL != "" {
		return map[string]any{"type": "remote", "url": server.URL, "headers": server.Headers, "enabled": true}
	}
	return map[string]any{"type": "local", "command": append([]string{server.Command}, server.Args...),
		"environment": server.Env, "enabled": true}
}

// codexTable is the servers as one TOML inline table. TOML's basic strings
// escape the way JSON's do, so each value is written as JSON.
func codexTable(servers []job.Server) string {
	entries := make([]string, 0, len(servers))
	for _, server := range servers {
		fields := []string{`default_tools_approval_mode="approve"`}
		if server.URL != "" {
			fields = append(fields, "url="+quoted(server.URL))
			if len(server.Headers) > 0 {
				fields = append(fields, "http_headers="+inlineTable(server.Headers))
			}
		} else {
			fields = append(fields, "command="+quoted(server.Command), "args="+quotedList(server.Args))
			if len(server.Env) > 0 {
				fields = append(fields, "env="+inlineTable(server.Env))
			}
		}
		entries = append(entries, quoted(server.Name)+"={"+strings.Join(fields, ",")+"}")
	}
	return "mcp_servers={" + strings.Join(entries, ",") + "}"
}

func inlineTable(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, quoted(key)+"="+quoted(values[key]))
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func quotedList(values []string) string {
	quotedEach := make([]string, 0, len(values))
	for _, value := range values {
		quotedEach = append(quotedEach, quoted(value))
	}
	return "[" + strings.Join(quotedEach, ",") + "]"
}

func quoted(value string) string {
	text, _ := json.Marshal(value)
	return string(text)
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func asJSON(value any) (string, error) {
	text, err := json.Marshal(value)
	return string(text), err
}
