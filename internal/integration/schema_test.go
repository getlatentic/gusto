package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The official schemas are the format's own account of what is valid. This reader is stricter in places, since the format also has
// rules in its text that a schema cannot say (https for any address but this computer, one executable in a command), so the check
// is one way: nothing the schema refuses may be taken, and everything the format shows as valid must be.

func official(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "schema", "agent-plugins", Version, name)
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document, err := jsonschema.UnmarshalJSON(strings.NewReader(string(text)))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(ecmaScript)
	if err := compiler.AddResource(path, document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(path)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func accepts(t *testing.T, schema *jsonschema.Schema, text string) bool {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
	if err != nil {
		return false
	}
	return schema.Validate(document) == nil
}

func TestNoServerTheSchemaRefusesIsTakenAndTheValidOnesAre(t *testing.T) {
	schema := official(t, "mcp.schema.json")
	for name, entry := range goodServers {
		_, text := server(entry)
		if !accepts(t, schema, text) {
			t.Errorf("%s: the schema refuses a server the tests call valid", name)
		}
	}
	refused := 0
	for name, entry := range badServers {
		_, text := server(entry)
		if accepts(t, schema, text) {
			continue
		}
		refused++
		plugin := read(t, map[string]string{"plugin.json": manifest(""), "mcp.json": text})
		if len(plugin.Servers) != 0 {
			t.Errorf("%s: the schema refuses it and the reader took it", name)
		}
	}
	if refused < 10 {
		t.Errorf("only %d of the bad servers are refused by the schema, so the check says little", refused)
	}
}

func TestNoManifestTheSchemaRefusesIsTaken(t *testing.T) {
	schema := official(t, "plugin.schema.json")
	manifests := []string{
		`{"name": "x"}`,
		`{` + pluginSchema + `}`,
		`{` + pluginSchema + `, "name": "My-Plugin"}`,
		`{` + pluginSchema + `, "name": "-start"}`,
		`{` + pluginSchema + `, "name": "has--double"}`,
		`{` + pluginSchema + `, "name": "too.many..dots"}`,
		`{` + pluginSchema + `, "name": ""}`,
		`{` + pluginSchema + `, "name": "` + strings.Repeat("a", 65) + `"}`,
		`{` + pluginSchema + `, "name": "x", "author": {"name": "a", "twitter": "b"}}`,
		`{` + pluginSchema + `, "name": "x", "keywords": [1]}`,
		`{` + pluginSchema + `, "name": "x", "version": 2}`,
		`{"$schema": "https://agent-plugins.org/schemas/1.1.0/plugin.schema.json", "name": "x"}`,
	}
	for _, text := range manifests {
		if accepts(t, schema, text) {
			t.Errorf("the schema takes %s, and the test means it to refuse", text)
			continue
		}
		if _, err := Read(folder(t, map[string]string{"plugin.json": text})); err == nil {
			t.Errorf("the schema refuses %s and the reader took it", text)
		}
	}
	for _, text := range []string{
		`{` + pluginSchema + `, "name": "minimal-plugin"}`,
		`{` + pluginSchema + `, "name": "acme.tools", "version": "1", "author": {"name": "a"}, "keywords": [], "extensions": {"a.b": {}}}`,
	} {
		if !accepts(t, schema, text) {
			t.Errorf("the schema refuses %s", text)
		}
		if _, err := Read(folder(t, map[string]string{"plugin.json": text})); err != nil {
			t.Errorf("the reader refuses %s: %v", text, err)
		}
	}
}

// ecmaScript is the regular expression flavour JSON Schema means. The plugin name's pattern needs a look-ahead, which Go's own
// package does not have.
type ecmaScriptPattern struct{ *regexp2.Regexp }

func (p ecmaScriptPattern) MatchString(text string) bool {
	matched, err := p.Regexp.MatchString(text)
	return err == nil && matched
}

func (p ecmaScriptPattern) String() string { return p.Regexp.String() }

func ecmaScript(pattern string) (jsonschema.Regexp, error) {
	compiled, err := regexp2.Compile(pattern, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	return ecmaScriptPattern{compiled}, nil
}
