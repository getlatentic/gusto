package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
)

var pluginName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

var manifestFields = []string{"$schema", "name", "version", "description", "author", "homepage", "repository", "license", "keywords", "extensions"}

// Read reads a plugin from its folder. It returns ErrRejected, with the reason, for a plugin it cannot take at all. Anything
// else wrong with it is in Problems, with the part that was left out.
func Read(dir string) (*Plugin, error) {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	files, size, escape, err := measure(root, root)
	switch {
	case err != nil:
		return nil, err
	case files > MaxFiles:
		return nil, rejected("it holds %d files, and %d is the most", files, MaxFiles)
	case size > MaxBytes:
		return nil, rejected("it holds %d MiB, and %d is the most", size>>20, MaxBytes>>20)
	}
	manifest := filepath.Join(root, "plugin.json")
	if !regular(root, manifest) {
		return nil, rejected("there is no plugin.json at its root")
	}
	plugin, err := readManifest(manifest)
	if err != nil {
		return nil, err
	}
	if escape != "" {
		plugin.note(escape, "a link that leaves the plugin's folder: nothing is read through it")
	}
	plugin.readSkills(root)
	plugin.readServers(root)
	return plugin, nil
}

func readManifest(path string) (*Plugin, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(text) > maxManifest {
		return nil, rejected("plugin.json is larger than %d KiB", maxManifest>>10)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(text, &fields); err != nil {
		return nil, rejected("plugin.json is not a JSON object: %v", err)
	}
	plugin := &Plugin{}
	var declared, name string
	if err := field(fields, "$schema", &declared); err != nil || declared == "" {
		return nil, rejected("plugin.json needs a $schema, the Agent Plugins version it is for (%s)", manifestSchema)
	}
	if declared != manifestSchema {
		return nil, rejected("plugin.json is for %s, and this reads only %s", declared, manifestSchema)
	}
	if err := field(fields, "name", &name); err != nil || !validName(name) {
		return nil, rejected("plugin.json needs a name of 1 to 64 characters: lower case letters, digits, hyphens and periods, starting and ending with a letter or digit, and no -- or ..")
	}
	plugin.Name = name
	for _, check := range []struct {
		key  string
		into any
	}{
		{"version", &plugin.Version}, {"description", &plugin.Description}, {"homepage", &plugin.Homepage},
		{"repository", &plugin.Repository}, {"license", &plugin.License}, {"keywords", &plugin.Keywords},
	} {
		if err := field(fields, check.key, check.into); err != nil {
			return nil, rejected("plugin.json: %v", err)
		}
	}
	if raw, there := fields["author"]; there {
		if err := strictAuthor(raw, &plugin.Author); err != nil {
			return nil, rejected("plugin.json: author %v", err)
		}
	}
	if raw, there := fields["extensions"]; there && !isObject(raw) {
		plugin.note("plugin.json", "extensions is not an object, and is ignored")
	}
	for key := range fields {
		if !slices.Contains(manifestFields, key) {
			plugin.note("plugin.json", "%s is not a field of the format, and is ignored", key)
		}
	}
	return plugin, nil
}

func validName(name string) bool {
	return len(name) >= 1 && len(name) <= 64 && pluginName.MatchString(name) && !contains(name, "--") && !contains(name, "..")
}

func contains(text, part string) bool { return bytes.Contains([]byte(text), []byte(part)) }

// field reads one field when it is there, and says when it is not the type it should be.
func field(fields map[string]json.RawMessage, key string, into any) error {
	raw, there := fields[key]
	if !there {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%s is the wrong kind of value", key)
	}
	return nil
}

func strictAuthor(raw json.RawMessage, into *Author) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("may hold only a name, an email and a url, each text: %v", err)
	}
	return nil
}

func isObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}
