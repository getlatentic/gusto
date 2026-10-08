package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
)

// serversFile is where a job's MCP server definitions wait in its setup
// directory, beside the skills it was given.
const serversFile = "servers.json"

// Servers are the MCP server definitions a job was given, or none.
func Servers(dir job.Dir) ([]job.Server, error) {
	text, err := os.ReadFile(filepath.Join(dir.SetupDir(), serversFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var servers []job.Server
	if err := json.Unmarshal(text, &servers); err != nil {
		return nil, err
	}
	return located(servers)
}

// GatewayName is what a harness calls the one server that fronts the servers behind the gateway.
const GatewayName = "gateway"

// withGateway gives a job that has servers behind the gateway the one server that reaches them: the proxy's, at the address the
// harness reaches its model by, called with the token that stands in for a credential. The definitions of the servers behind it
// are not here: they stay with the proxy.
func withGateway(servers []job.Server, spec job.Spec) ([]job.Server, error) {
	more, err := gatewayServers(spec, harness.ModelURL(), harness.ModelToken())
	return append(servers, more...), err
}

// gatewayServers is the one server that reaches a job's servers behind the gateway, for a harness that reaches its model at modelURL
// with token, or none when the job has none behind it. The gateway is at the same place as the model, and takes the same token.
func gatewayServers(spec job.Spec, modelURL, token string) ([]job.Server, error) {
	if spec.Setup == nil || len(spec.Setup.Gateway) == 0 {
		return nil, nil
	}
	if token == "" {
		return nil, fmt.Errorf("the setup %s reaches %d servers through the gateway, which is part of the proxy, and this run has no proxy",
			spec.Setup.Name, len(spec.Setup.Gateway))
	}
	root := strings.TrimSuffix(strings.TrimRight(modelURL, "/"), "/v1")
	return []job.Server{{Name: GatewayName, URL: root + "/mcp", Headers: map[string]string{"Authorization": "Bearer " + token}}}, nil
}

// located names each server's program by its full path, found on this box's
// PATH. ACP asks for one, and a program the image does not carry is better
// refused here, by name, than failing inside the harness where only its log
// would say so.
func located(servers []job.Server) ([]job.Server, error) {
	for i, server := range servers {
		if server.Command == "" {
			continue
		}
		path, err := exec.LookPath(server.Command)
		if err != nil {
			return nil, fmt.Errorf("the MCP server %s runs %s, which this runner does not carry: "+
				"add its package to the image's MCP_SERVERS", server.Name, server.Command)
		}
		servers[i].Command = path
	}
	return servers, nil
}

// fits refuses a setup the harness cannot take: skills for one that finds
// none, or servers for one with no MCP. Starting it anyway would run a bare
// harness under the name of a configured one.
func fits(spec job.Spec, adapter harness.Adapter) error {
	if spec.Setup == nil {
		return nil
	}
	if len(spec.Setup.Skills) > 0 && adapter.Skills == "" {
		return fmt.Errorf("%s takes no skills, and the setup %s gives it %d", adapter.Name, spec.Setup.Name, len(spec.Setup.Skills))
	}
	if served := len(spec.Setup.Servers) + len(spec.Setup.Gateway); served > 0 && !adapter.TakesMCP() {
		return fmt.Errorf("%s takes no MCP servers, and the setup %s gives it %d", adapter.Name, spec.Setup.Name, served)
	}
	return nil
}

// placeSkills copies a job's skills into the folder its harness finds them
// in, each into a folder of its own name. Only those folders are replaced, so
// a skill edited since the last run is the new one, and nothing else in the
// harness's folder is touched: an adapter travels with a job and can name any
// path, and on a runner without a container that path is the host's.
func placeSkills(dir job.Dir, spec job.Spec, adapter harness.Adapter) error {
	if spec.Setup == nil || adapter.Skills == "" {
		return nil
	}
	from := filepath.Join(dir.SetupDir(), "skills")
	for _, name := range spec.Setup.Skills {
		if !job.Named(name) {
			return fmt.Errorf("%q is not a skill's name", name)
		}
		to := filepath.Join(adapter.Skills, name)
		if err := os.RemoveAll(to); err != nil {
			return err
		}
		if err := os.CopyFS(to, os.DirFS(filepath.Join(from, name))); err != nil {
			return err
		}
	}
	return nil
}
