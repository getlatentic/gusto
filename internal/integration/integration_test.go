package integration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pluginSchema = `"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"`
	mcpHeader    = `"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"`
)

// folder makes a plugin folder from files by path. A value that starts with "-> " is a link to the path after it.
func folder(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if target, link := strings.CutPrefix(text, "-> "); link {
			if err := os.Symlink(target, full); err != nil {
				t.Skip("no links here:", err)
			}
			continue
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func skillText(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\nDo the thing.\n"
}

func manifest(extra string) string {
	return "{" + pluginSchema + `, "name": "reports"` + extra + "}"
}

func read(t *testing.T, files map[string]string) *Plugin {
	t.Helper()
	plugin, err := Read(folder(t, files))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return plugin
}

func left(p *Plugin) []string {
	var where []string
	for _, problem := range p.Problems {
		if problem.Left {
			where = append(where, problem.Where)
		}
	}
	return where
}

func TestTheSpecsMinimalAndFullManifestsAreRead(t *testing.T) {
	minimal := read(t, map[string]string{"plugin.json": `{` + pluginSchema + `, "name": "minimal-plugin"}`})
	if minimal.Name != "minimal-plugin" || len(minimal.Problems) != 0 {
		t.Fatalf("%+v", minimal)
	}
	full := read(t, map[string]string{"plugin.json": `{` + pluginSchema + `,
		"name": "plugin-name", "version": "1.2.0", "description": "Brief plugin description",
		"author": {"name": "Author Name", "email": "author@example.com", "url": "https://example.com"},
		"homepage": "https://docs.example.com/plugin", "repository": "https://github.com/example/plugin", "license": "MIT",
		"keywords": ["keyword1", "keyword2"], "extensions": {"com.example.client": {"setting": true}}}`})
	if full.Version != "1.2.0" || full.Author.Email != "author@example.com" || len(full.Keywords) != 2 || len(full.Problems) != 0 {
		t.Fatalf("%+v", full)
	}
}

func TestTheSpecsThreeKindsOfServerAreRead(t *testing.T) {
	plugin := read(t, map[string]string{
		"plugin.json": manifest(""),
		"mcp.json": `{` + mcpHeader + `, "mcpServers": {
			"local-validator": {"type": "stdio", "command": "./bin/validator", "args": ["--data", "${PLUGIN_DATA}/validator"],
				"env": {"CONFIG": "${PLUGIN_ROOT}/config.json"}, "cwd": "${PLUGIN_ROOT}"},
			"deployment-api": {"type": "streamable-http", "url": "https://deploy.example.com/mcp", "headers": {"X-Tenant": "public-tenant"}},
			"legacy-events": {"type": "sse", "url": "https://legacy.example.com/sse"}}}`,
	})
	if len(plugin.Servers) != 3 || len(plugin.Problems) != 0 {
		t.Fatalf("%+v", plugin)
	}
	by := map[string]Server{}
	for _, server := range plugin.Servers {
		by[server.Name] = server
	}
	if got := by["deployment-api"]; got.Transport != StreamableHTTP || got.Headers["X-Tenant"] != "public-tenant" {
		t.Fatalf("%+v", got)
	}
	if by["legacy-events"].Transport != SSE || by["local-validator"].Args[1] != "${PLUGIN_DATA}/validator" || by["local-validator"].Cwd != "${PLUGIN_ROOT}" {
		t.Error("placeholders are kept as written, to be expanded by what runs the server")
	}
}

func TestAManifestThatIsWrongRejectsThePlugin(t *testing.T) {
	cases := map[string]map[string]string{
		"no plugin.json":                   {"skills/a/SKILL.md": skillText("a", "d")},
		"not JSON":                         {"plugin.json": `{`},
		"no $schema":                       {"plugin.json": `{"name": "x"}`},
		"the draft 1.1.0":                  {"plugin.json": `{"$schema": "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json", "name": "x"}`},
		"no name":                          {"plugin.json": `{` + pluginSchema + `}`},
		"upper case name":                  {"plugin.json": `{` + pluginSchema + `, "name": "My-Plugin"}`},
		"leading hyphen":                   {"plugin.json": `{` + pluginSchema + `, "name": "-start"}`},
		"double hyphen":                    {"plugin.json": `{` + pluginSchema + `, "name": "has--double"}`},
		"double period":                    {"plugin.json": `{` + pluginSchema + `, "name": "too.many..dots"}`},
		"a name of 65":                     {"plugin.json": `{` + pluginSchema + `, "name": "` + strings.Repeat("a", 65) + `"}`},
		"author with extra":                {"plugin.json": manifest(`, "author": {"name": "a", "twitter": "b"}`)},
		"keywords of numbers":              {"plugin.json": manifest(`, "keywords": [1]`)},
		"version not text":                 {"plugin.json": manifest(`, "version": 2`)},
		"a plugin.json that is a link out": {"plugin.json": "-> /etc/hostname"},
	}
	for name, files := range cases {
		if _, err := Read(folder(t, files)); !errors.Is(err, ErrRejected) {
			t.Errorf("%s: err = %v, want the plugin rejected", name, err)
		}
	}
}

func TestValidNamesAreTakenIncludingPeriods(t *testing.T) {
	for _, name := range []string{"my-plugin", "acme.tools", "lint3r", "a"} {
		if _, err := Read(folder(t, map[string]string{"plugin.json": `{` + pluginSchema + `, "name": "` + name + `"}`})); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAFieldTheFormatDoesNotHaveIsSaidAndIgnored(t *testing.T) {
	plugin := read(t, map[string]string{"plugin.json": manifest(`, "mcpServers": {}, "extensions": 5`)})
	if plugin.Name != "reports" || len(plugin.Problems) != 2 || len(left(plugin)) != 0 {
		t.Fatalf("%+v", plugin.Problems)
	}
}

func TestSkillsAreFoundOneFolderDeepAndABadOneIsLeftOut(t *testing.T) {
	plugin := read(t, map[string]string{
		"plugin.json":                      manifest(""),
		"skills/summarize/SKILL.md":        skillText("summarize", "Summarise a report."),
		"skills/summarize/scripts/run.sh":  "echo",
		"skills/summarize/references/a.md": "x",
		"skills/wrong-name/SKILL.md":       skillText("other", "d"),
		"skills/no-description/SKILL.md":   "---\nname: no-description\n---\n",
		"skills/no-front/SKILL.md":         "just words",
		"skills/Upper/SKILL.md":            skillText("Upper", "d"),
		"skills/group/inner/SKILL.md":      skillText("inner", "d"),
		"skills/no-file/readme.md":         "x",
		"skills/meta/SKILL.md":             "---\nname: meta\ndescription: >\n  Folded\n  text.\nmetadata:\n  version: 1.0\n  author: a\nlicense: MIT\nallowed-tools: Bash(git:*)\n---\n",
	})
	var names []string
	for _, skill := range plugin.Skills {
		names = append(names, skill.Name)
	}
	if strings.Join(names, ",") != "meta,summarize" {
		t.Fatalf("skills = %v", names)
	}
	if got := strings.Join(left(plugin), ","); got != "skills/Upper,skills/no-description,skills/no-front,skills/wrong-name" {
		t.Errorf("left out = %s", got)
	}
	meta := plugin.Skills[0]
	if meta.Description != "Folded text.\n" && meta.Description != "Folded text." || meta.Metadata["version"] != "1" && meta.Metadata["version"] != "1.0" || meta.License != "MIT" || meta.AllowedTools != "Bash(git:*)" {
		t.Errorf("%+v", meta)
	}
	if plugin.Skills[1].Dir != "skills/summarize" {
		t.Errorf("dir = %s", plugin.Skills[1].Dir)
	}
}

func TestALinkThatLeavesThePluginTakesNothingThroughIt(t *testing.T) {
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte(skillText("stolen", "d")), 0o644)
	plugin := read(t, map[string]string{
		"plugin.json":            manifest(""),
		"skills/ok/SKILL.md":     skillText("ok", "d"),
		"skills/stolen":          "-> " + outside,
		"skills/sneaky/SKILL.md": skillText("sneaky", "d"),
		"skills/sneaky/data":     "-> /etc",
		"skills/inside/SKILL.md": skillText("inside", "d"),
		"skills/inside/copy.md":  "-> SKILL.md",
		"mcp.json":               "-> /etc/hosts",
	})
	var names []string
	for _, skill := range plugin.Skills {
		names = append(names, skill.Name)
	}
	if strings.Join(names, ",") != "inside,ok" {
		t.Errorf("skills = %v, problems = %v", names, plugin.Problems)
	}
	if len(plugin.Servers) != 0 || !strings.Contains(strings.Join(left(plugin), ","), "mcp.json") {
		t.Errorf("an mcp.json that is a link out was read: %v", plugin.Problems)
	}
}

func TestAPluginThatIsTooLargeIsRejected(t *testing.T) {
	files := map[string]string{"plugin.json": manifest("")}
	for i := range MaxFiles + 1 {
		files["assets/"+strings.Repeat("d/", i%5)+"f"+itoa(i)] = "x"
	}
	if _, err := Read(folder(t, files)); !errors.Is(err, ErrRejected) {
		t.Errorf("err = %v", err)
	}
	deep := map[string]string{"plugin.json": manifest(""), strings.Repeat("d/", MaxDepth+2) + "f": "x"}
	if _, err := Read(folder(t, deep)); !errors.Is(err, ErrRejected) {
		t.Errorf("a deep plugin: err = %v", err)
	}
}

func itoa(n int) string {
	digits := ""
	for ; n > 0; n /= 10 {
		digits = string(rune('0'+n%10)) + digits
	}
	return "n" + digits
}

func server(entry string) (*Plugin, string) {
	return nil, `{` + mcpHeader + `, "mcpServers": {"s": ` + entry + `}}`
}

var badServers = map[string]string{
	"no type":                 `{"command": "x"}`,
	"unknown type":            `{"type": "websocket", "url": "https://a.example/mcp"}`,
	"unknown field":           `{"type": "stdio", "command": "x", "shell": true}`,
	"stdio with a url":        `{"type": "stdio", "command": "x", "url": "https://a.example"}`,
	"http with a command":     `{"type": "streamable-http", "url": "https://a.example", "command": "x"}`,
	"no command":              `{"type": "stdio"}`,
	"a command line":          `{"type": "stdio", "command": "npx -y thing"}`,
	"a command with a path":   `{"type": "stdio", "command": "bin/server"}`,
	"an absolute command":     `{"type": "stdio", "command": "/usr/bin/x"}`,
	"a command that climbs":   `{"type": "stdio", "command": "./../x"}`,
	"a command with a hole":   `{"type": "stdio", "command": "${PLUGIN_ROOT}/x"}`,
	"cwd that is the parent":  `{"type": "stdio", "command": "x", "cwd": "${PLUGIN_DATA}/.."}`,
	"cwd with no prefix":      `{"type": "stdio", "command": "x", "cwd": "data"}`,
	"cwd that climbs":         `{"type": "stdio", "command": "x", "cwd": "${PLUGIN_ROOT}/../x"}`,
	"cwd of another variable": `{"type": "stdio", "command": "x", "cwd": "${PLUGIN_ROOTS}"}`,
	"cwd absolute":            `{"type": "stdio", "command": "x", "cwd": "/tmp"}`,
	"env sets the reserved":   `{"type": "stdio", "command": "x", "env": {"PLUGIN_ROOT": "/"}}`,
	"env sets it in lower":    `{"type": "stdio", "command": "x", "env": {"plugin_data": "/"}}`,
	"args not text":           `{"type": "stdio", "command": "x", "args": [1]}`,
	"http, not this computer": `{"type": "streamable-http", "url": "http://a.example/mcp"}`,
	"a relative url":          `{"type": "streamable-http", "url": "/mcp"}`,
	"a url with a password":   `{"type": "streamable-http", "url": "https://u:p@a.example/mcp"}`,
	"a url with a fragment":   `{"type": "streamable-http", "url": "https://a.example/mcp#x"}`,
	"a url of another scheme": `{"type": "sse", "url": "ftp://a.example/mcp"}`,
	"no url":                  `{"type": "sse"}`,
	"a header twice":          `{"type": "sse", "url": "https://a.example", "headers": {"X-A": "1", "x-a": "2"}}`,
	"a header with a break":   `{"type": "sse", "url": "https://a.example", "headers": {"X-A": "1\r\nX-B: 2"}}`,
	"a bad header name":       `{"type": "sse", "url": "https://a.example", "headers": {"X A": "1"}}`,
}

var goodServers = map[string]string{
	"a bare command":          `{"type": "stdio", "command": "npx", "args": ["-y", "thing"]}`,
	"a command in the plugin": `{"type": "stdio", "command": "./bin/x", "cwd": "./data"}`,
	"cwd under data":          `{"type": "stdio", "command": "x", "cwd": "${PLUGIN_DATA}/work"}`,
	"http on this computer":   `{"type": "streamable-http", "url": "http://localhost:8080/mcp"}`,
	"http on 127.0.0.1":       `{"type": "streamable-http", "url": "http://127.0.0.1:8080/mcp"}`,
	"http on ::1":             `{"type": "sse", "url": "http://[::1]:8080/sse"}`,
}

func TestEachKindOfBadServerIsLeftOutAndTheGoodOnesStay(t *testing.T) {
	for name, entry := range badServers {
		_, text := server(entry)
		plugin := read(t, map[string]string{"plugin.json": manifest(""), "mcp.json": text})
		if len(plugin.Servers) != 0 || len(left(plugin)) != 1 {
			t.Errorf("%s: the server was taken: %+v", name, plugin)
		}
	}
	for name, entry := range goodServers {
		_, text := server(entry)
		plugin := read(t, map[string]string{"plugin.json": manifest(""), "mcp.json": text})
		if len(plugin.Servers) != 1 {
			t.Errorf("%s: the server was left out: %v", name, plugin.Problems)
		}
	}
}

func TestAnMCPFileThatIsWrongHoldsNoServersAndTheSkillsStay(t *testing.T) {
	cases := map[string]string{
		"not JSON":                 `{`,
		"a different version":      `{"$schema": "https://agent-plugins.org/schemas/1.1.0/mcp.schema.json", "mcpServers": {}}`,
		"no $schema":               `{"mcpServers": {}}`,
		"an extra top-level field": `{` + mcpHeader + `, "mcpServers": {}, "inputs": []}`,
		"servers as a list":        `{` + mcpHeader + `, "mcpServers": []}`,
	}
	for name, text := range cases {
		plugin := read(t, map[string]string{"plugin.json": manifest(""), "mcp.json": text, "skills/a/SKILL.md": skillText("a", "d")})
		if len(plugin.Servers) != 0 || len(plugin.Skills) != 1 || strings.Join(left(plugin), ",") != "mcp.json" {
			t.Errorf("%s: %+v", name, plugin)
		}
	}
	empty := read(t, map[string]string{"plugin.json": manifest(""), "mcp.json": `{` + mcpHeader + `, "mcpServers": {}}`})
	if len(empty.Problems) != 0 {
		t.Errorf("an empty mcpServers is valid: %v", empty.Problems)
	}
}
