package harness

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getlatentic/gusto/internal/job"
)

// SetupFolder is where the task's setup sits in the listing a digest is taken
// over: its skills and its MCP server definitions, beside the agent's own
// files. An agent directory may not hold a folder of that name, so one path
// in the listing never means two files.
const SetupFolder = "setup"

// setupOf reads the setup the job carries, or nothing when it carries none or
// there is no job.
func setupOf(dir job.Dir) (Tree, error) {
	if dir == "" || !isDir(dir.SetupDir()) {
		return nil, nil
	}
	tree, err := walk(dir.SetupDir())
	if err != nil {
		return nil, err
	}
	for i, file := range tree {
		if file.Path == serversFile {
			if tree[i].Content, err = nameOnly(file.Content); err != nil {
				return nil, err
			}
		}
	}
	return tree, nil
}

// serversFile is where a setup's MCP server definitions sit.
const serversFile = "servers.json"

// nameOnly is the server definitions with the value of every environment
// variable and header left out and its name kept. Those are where a token
// goes, and a digest that took them would call one setup two harnesses the
// day a credential rotated, and two teams running the same setup on their
// own credentials would never compare. The names stay, because a server given
// a different variable is configured differently.
func nameOnly(definitions []byte) ([]byte, error) {
	var servers []job.Server
	if err := json.Unmarshal(definitions, &servers); err != nil {
		return nil, fmt.Errorf("setup/%s: %w", serversFile, err)
	}
	for i := range servers {
		servers[i].Env = blanked(servers[i].Env)
		servers[i].Headers = blanked(servers[i].Headers)
	}
	return json.Marshal(servers)
}

func blanked(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	names := make(map[string]string, len(values))
	for name := range values {
		names[name] = ""
	}
	return names
}

// with is the tree and a setup together, the setup's files under setup/, in
// the path order a digest needs.
func (t Tree) with(setup Tree) Tree {
	joined := append(Tree{}, t...)
	for _, file := range setup {
		file.Path = SetupFolder + "/" + file.Path
		joined = append(joined, file)
	}
	joined.sort()
	return joined
}

func (t Tree) holdsSetup() bool {
	for _, file := range t {
		if strings.HasPrefix(file.Path, SetupFolder+"/") {
			return true
		}
	}
	return false
}
