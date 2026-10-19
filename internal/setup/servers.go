package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getlatentic/gusto/internal/job"
)

// Servers is every MCP server the deployment offers, by name.
type Servers map[string]job.Server

// OpenServers reads one file a server from dir, named for the server: a
// program the box runs, {"command", "args", "env"}, or an address it calls,
// {"url", "headers"}. No directory is a deployment offering none.
//
// A program has to be in the runner image, since the box fetches nothing, and
// an address has to be one the proxy lets a task reach.
func OpenServers(dir string) (Servers, error) {
	offered := Servers{}
	if dir == "" {
		return offered, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return offered, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name, isJSON := strings.CutSuffix(entry.Name(), ".json")
		if !isJSON || entry.IsDir() {
			continue
		}
		server, err := readServer(filepath.Join(dir, entry.Name()), name)
		if err != nil {
			return nil, err
		}
		offered[name] = server
	}
	return offered, nil
}

func readServer(path, name string) (job.Server, error) {
	if !job.Named(name) {
		return job.Server{}, fmt.Errorf("%s: %q cannot name an MCP server", path, name)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return job.Server{}, err
	}
	var server job.Server
	if err := json.Unmarshal(text, &server); err != nil {
		return job.Server{}, fmt.Errorf("%s is not an MCP server: %w", path, err)
	}
	server.Name = name
	if (server.Command == "") == (server.URL == "") {
		return job.Server{}, fmt.Errorf("%s names neither a command nor a url, or both", path)
	}
	return server, nil
}

// Names is the servers offered, in order.
func (s Servers) Names() []string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
