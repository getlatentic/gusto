package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	tools "github.com/getlatentic/haas/internal/gateway"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/setup"
)

// gateways makes a job's gateway the first time it is asked for, from what the job was given when it started, and ends it with the
// job. What a job may reach is read from its own directory, which only its owner and the proxy can read, and not from anything the
// harness sends.
type gateways struct {
	jobs string
	// policy is the tool policy as it is now, which a change to its file replaces and a job that is running asks for at each call.
	policy  atomic.Pointer[tools.Policy]
	mode    tools.Mode
	private bool

	mu       sync.Mutex
	sessions map[string]*tools.Session
}

// openGateways reads how the gateway is configured: HAAS_TOOL_POLICY is the policy file, HAAS_GATEWAY_MODE is discover or all, and
// HAAS_GATEWAY_PRIVATE=1 lets the servers be on a private network.
func openGateways(jobsRoot string) (*gateways, error) {
	policy, err := tools.ReadPolicy(os.Getenv("HAAS_TOOL_POLICY"))
	if err != nil {
		return nil, err
	}
	mode := tools.Mode(os.Getenv("HAAS_GATEWAY_MODE"))
	if mode != tools.All {
		mode = tools.Discover
	}
	g := &gateways{jobs: jobsRoot, mode: mode, private: os.Getenv("HAAS_GATEWAY_PRIVATE") == "1", sessions: map[string]*tools.Session{}}
	g.policy.Store(&policy)
	return g, nil
}

// reload reads the policy file again, and keeps the policy it has when the file does not read.
func (g *gateways) reload() error {
	policy, err := tools.ReadPolicy(os.Getenv("HAAS_TOOL_POLICY"))
	if err != nil {
		return err
	}
	g.policy.Store(&policy)
	return nil
}

func (g *gateways) currentPolicy() tools.Policy { return *g.policy.Load() }

// For is the gateway of a job, or false when the job has no servers behind it.
func (g *gateways) For(id string) (http.Handler, bool) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return nil, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if made, there := g.sessions[id]; there {
		return made, true
	}
	made, ok := g.open(id)
	if ok {
		g.sessions[id] = made
	}
	return made, ok
}

func (g *gateways) open(id string) (*tools.Session, bool) {
	dir := job.Dir(filepath.Join(g.jobs, id))
	spec, err := job.Read(dir)
	if err != nil || spec.Setup == nil || len(spec.Setup.Gateway) == 0 {
		return nil, false
	}
	text, err := os.ReadFile(filepath.Join(dir.SetupDir(), setup.GatewayFile))
	if err != nil {
		return nil, false
	}
	var defined []tools.Server
	if json.Unmarshal(text, &defined) != nil {
		return nil, false
	}
	named := map[string]bool{}
	for _, name := range spec.Setup.Gateway {
		named[name] = true
	}
	var servers []tools.Server
	for _, server := range defined {
		if named[server.Name] {
			servers = append(servers, server)
		}
	}
	caller := tools.Caller{Job: id, Team: spec.Team, Person: askerOf(dir)}
	return tools.New(tools.Config{
		Caller: caller, Servers: servers, Live: g.currentPolicy, Mode: g.mode, Private: g.private,
		Record: tools.OpenRecord(dir.OutFile(tools.RecordFile)),
	}), true
}

// askerOf is who asked for the job, from its state file.
func askerOf(dir job.Dir) string {
	text, err := os.ReadFile(filepath.Join(dir.Path(), "state.json"))
	if err != nil {
		return ""
	}
	var state struct {
		Asked string `json:"asked"`
	}
	_ = json.Unmarshal(text, &state)
	return state.Asked
}

// Forget ends a job's gateway, and its sessions with the servers.
func (g *gateways) Forget(id string) {
	g.mu.Lock()
	made, there := g.sessions[id]
	delete(g.sessions, id)
	g.mu.Unlock()
	if there {
		made.Close()
	}
}
