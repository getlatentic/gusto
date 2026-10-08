package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// readServers reads mcp.json: one entry at a time, so that a bad server does not take the good ones with it.
func (p *Plugin) readServers(root string) {
	file := filepath.Join(root, "mcp.json")
	if _, err := os.Lstat(file); err != nil {
		return
	}
	if !regular(root, file) {
		p.leave("mcp.json", "it is not a file inside the plugin, so it holds no servers")
		return
	}
	text, err := os.ReadFile(file)
	if err != nil || len(text) > maxManifest {
		p.leave("mcp.json", "it cannot be read, or is larger than %d KiB, so it holds no servers", maxManifest>>10)
		return
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(text, &top); err != nil {
		p.leave("mcp.json", "it is not a JSON object, so it holds no servers")
		return
	}
	var declared string
	_ = json.Unmarshal(top["$schema"], &declared)
	if declared != mcpSchema {
		p.leave("mcp.json", "its $schema is %q, and this plugin and this reader are for %s", declared, mcpSchema)
		return
	}
	var servers map[string]json.RawMessage
	if len(top) != 2 || json.Unmarshal(top["mcpServers"], &servers) != nil || servers == nil {
		p.leave("mcp.json", "it must hold a $schema and mcpServers, an object, and nothing else")
		return
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		server, problem := readServer(name, servers[name])
		if problem != "" {
			p.leave("mcp.json: server "+name, "%s", problem)
			continue
		}
		p.Servers = append(p.Servers, server)
	}
}

// entry is a server as written, with the fields of every kind, so that a field of the wrong kind is told from one that is unknown.
type entry struct {
	Type    Transport         `json:"type"`
	Command *string           `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     *string           `json:"cwd"`
	URL     *string           `json:"url"`
	Headers map[string]string `json:"headers"`
}

func readServer(name string, raw json.RawMessage) (Server, string) {
	var written entry
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&written); err != nil {
		return Server{}, "it is not a server of the format: " + err.Error()
	}
	server := Server{Name: name, Transport: written.Type, Args: written.Args, Env: written.Env, Headers: written.Headers}
	switch written.Type {
	case Stdio:
		if written.URL != nil || written.Headers != nil {
			return Server{}, "a stdio server has no url or headers"
		}
		return stdio(server, written)
	case StreamableHTTP, SSE:
		if written.Command != nil || written.Args != nil || written.Env != nil || written.Cwd != nil {
			return Server{}, "a " + string(written.Type) + " server has no command, args, env or cwd"
		}
		return remote(server, written)
	}
	return Server{}, fmt.Sprintf("its type %q is not stdio, streamable-http or sse", written.Type)
}

func stdio(server Server, written entry) (Server, string) {
	if written.Command == nil || *written.Command == "" {
		return Server{}, "a stdio server needs a command"
	}
	command := *written.Command
	if problem := checkCommand(command); problem != "" {
		return Server{}, problem
	}
	for key := range written.Env {
		if strings.EqualFold(key, "PLUGIN_ROOT") || strings.EqualFold(key, "PLUGIN_DATA") {
			return Server{}, key + " is set by the client and cannot be set in env"
		}
	}
	if written.Cwd != nil {
		if problem := checkCwd(*written.Cwd); problem != "" {
			return Server{}, problem
		}
		server.Cwd = *written.Cwd
	}
	server.Command = command
	return server, ""
}

// checkCommand takes one executable: a bare name, or a path in the plugin that begins ./ . It is never a shell command line.
func checkCommand(command string) string {
	if strings.HasPrefix(command, "./") {
		if !contained(strings.TrimPrefix(command, "./")) {
			return "its command leaves the plugin's folder"
		}
		return ""
	}
	if strings.ContainsAny(command, " \t\r\n/\\") || strings.Contains(command, "${") {
		return "its command must be one executable: a bare name, or a path in the plugin that begins ./"
	}
	return ""
}

// checkCwd takes ./path, ${PLUGIN_ROOT} or ${PLUGIN_DATA}, and a path under one of them that does not climb out of it.
func checkCwd(cwd string) string {
	for _, prefix := range []string{"./", "${PLUGIN_ROOT}", "${PLUGIN_DATA}"} {
		rest, found := strings.CutPrefix(cwd, prefix)
		if !found {
			continue
		}
		if prefix != "./" && rest != "" && !strings.HasPrefix(rest, "/") {
			continue
		}
		if !contained(strings.TrimPrefix(rest, "/")) {
			return "its cwd leaves the folder it is in"
		}
		return ""
	}
	return "its cwd must begin ./, ${PLUGIN_ROOT} or ${PLUGIN_DATA}"
}

// contained says whether a path written in a plugin stays under its folder when its dots are taken out. Links are not looked at:
// nothing here runs the path, and what does must resolve it again.
func contained(written string) bool {
	clean := path.Clean(strings.ReplaceAll(written, "\\", "/"))
	return !strings.Contains(written, "\x00") && !path.IsAbs(clean) && clean != ".." && !strings.HasPrefix(clean, "../")
}

func remote(server Server, written entry) (Server, string) {
	if written.URL == nil {
		return Server{}, "a " + string(written.Type) + " server needs a url"
	}
	if problem := checkURL(*written.URL); problem != "" {
		return Server{}, problem
	}
	seen := map[string]bool{}
	for name := range written.Headers {
		if !validHeaderName(name) {
			return Server{}, fmt.Sprintf("%q is not a header name", name)
		}
		if lower := strings.ToLower(name); seen[lower] {
			return Server{}, "the header " + name + " is given twice"
		} else {
			seen[lower] = true
		}
	}
	for _, value := range written.Headers {
		if strings.ContainsAny(value, "\r\n\x00") {
			return Server{}, "a header value holds a line break"
		}
	}
	server.URL = *written.URL
	return server, ""
}

// checkURL takes an absolute http or https address with no user and no fragment, and https for anything but this computer.
func checkURL(address string) string {
	parsed, err := url.Parse(address)
	switch {
	case err != nil || !parsed.IsAbs() || parsed.Host == "":
		return "its url must be absolute"
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		return "its url must be http or https"
	case parsed.User != nil:
		return "its url must not hold a user name or password"
	case parsed.Fragment != "" || strings.Contains(address, "#"):
		return "its url must not hold a fragment"
	case parsed.Scheme == "http" && !loopback(parsed.Hostname()):
		return "its url must be https, since it is not this computer"
	}
	return ""
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validHeaderName is an HTTP token: letters, digits and these marks, and nothing else.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}
