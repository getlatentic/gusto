// Package integration reads an integration packaged in the open Agent Plugins format (https://agent-plugins.org, version
// 1.0.0): a folder with a plugin.json, Agent Skills under skills/, and MCP servers in mcp.json.
//
// Reading follows the specification's failure boundaries. A manifest that is wrong rejects the plugin. A skill or a server that
// is wrong is left out and said so, and the rest of the plugin is kept, so that one bad entry does not stop an organization
// from taking the others. The plugin is untrusted input: nothing in it is run here, and a path that leaves its folder is refused.
package integration

import (
	"errors"
	"fmt"
)

// Format is the one Agent Plugins version this reads. Version 1.1.0 is a working draft and is not read.
const Format = "1.0.0"

const (
	manifestSchema = "https://agent-plugins.org/schemas/" + Format + "/plugin.schema.json"
	mcpSchema      = "https://agent-plugins.org/schemas/" + Format + "/mcp.schema.json"
)

// Plugin is what was read from a plugin folder.
type Plugin struct {
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Author      Author   `json:"author,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	License     string   `json:"license,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Skills      []Skill  `json:"skills,omitempty"`
	Servers     []Server `json:"servers,omitempty"`
	// Problems is everything that was left out or ignored, and why.
	Problems []Problem `json:"problems,omitempty"`
}

// Author is who made the plugin.
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Skill is one Agent Skill found under skills/.
type Skill struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	AllowedTools  string            `json:"allowedTools,omitempty"`
	// Dir is the skill's folder, from the plugin root: skills/<name>.
	Dir string `json:"dir"`
}

// Transport names how a server is reached.
type Transport string

const (
	Stdio          Transport = "stdio"
	StreamableHTTP Transport = "streamable-http"
	SSE            Transport = "sse"
)

// Server is one MCP server from mcp.json. Placeholders such as ${PLUGIN_ROOT} are kept as written: they are expanded by whatever
// runs the server.
type Server struct {
	Name      string            `json:"name"`
	Transport Transport         `json:"type"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// Problem is a thing the plugin does wrong, or that is left alone.
type Problem struct {
	// Where is the file or entry it is about, such as "mcp.json: server db" or "skills/summarize".
	Where   string `json:"where"`
	Message string `json:"message"`
	// Left says the component was left out. A problem that does not leave anything out is a note.
	Left bool `json:"left,omitempty"`
}

func (p Problem) String() string { return p.Where + ": " + p.Message }

// ErrRejected is a plugin that cannot be taken at all: its manifest is missing or wrong, or it is too large, or a path of it leaves
// the folder.
var ErrRejected = errors.New("not an Agent Plugin this can take")

func rejected(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRejected, fmt.Sprintf(format, args...))
}

func (p *Plugin) leave(where, format string, args ...any) {
	p.Problems = append(p.Problems, Problem{Where: where, Message: fmt.Sprintf(format, args...), Left: true})
}

func (p *Plugin) note(where, format string, args ...any) {
	p.Problems = append(p.Problems, Problem{Where: where, Message: fmt.Sprintf(format, args...)})
}
