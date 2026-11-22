package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/node"
	"github.com/getlatentic/gusto/internal/settings"
)

// nodeSettings is what gusto node reads: what this node is called, where it listens and how it knows gusto cloud.
type nodeSettings struct {
	name, private, secretFile, nodesDir string
	port                                int
}

func (h *nodeSettings) declare(set *settings.Set) {
	set.String(&h.name, "GUSTO_NODE_NAME", "", "what this node is called: lower case letters, digits and hyphens, up to 31")
	set.String(&h.private, "GUSTO_NODE_PRIVATE", "", "this node's IPv4 address on the private network, which its servers bind to")
	set.Int(&h.port, "GUSTO_NODE_PORT", 8060, 1, "the port to listen on")
	set.String(&h.secretFile, "GUSTO_NODE_SECRET_FILE", "", "the file with the secret gusto cloud and the nodes sign with")
	set.String(&h.nodesDir, "GUSTO_NODES_DIR", "", "the directory every node and gusto cloud see")
	renamedFromCell(set, "GUSTO_NODE_NAME", "GUSTO_NODE_PRIVATE", "GUSTO_NODE_PORT", "GUSTO_NODE_SECRET_FILE", "GUSTO_NODES_DIR")
}

// nodeCommand runs the agent on a node that runs organizations' servers for a gusto cloud on another
// node. It starts and stops what gusto cloud asks, signed with the secret they share, and says every few
// seconds that it is there.
func nodeCommand() int {
	var cfg nodeSettings
	if err := loadSettings(cfg.declare); err != nil {
		return refused(err)
	}
	agent, listen, err := nodeOf(cfg)
	if err != nil {
		return refused(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := agent.Run(ctx, listen, cfg.nodesDir, 10*time.Second); err != nil {
		return fail(err)
	}
	return 0
}

// nodeOf is the agent a node runs, and the address it listens on, or why the node's settings cannot make one.
func nodeOf(cfg nodeSettings) (agent *node.Agent, listen string, err error) {
	if !node.ValidName(cfg.name) {
		return nil, "", errors.New("GUSTO_NODE_NAME is what this node is called: lower case letters, digits and hyphens, up to 31")
	}
	if ip := net.ParseIP(cfg.private); ip == nil || ip.To4() == nil {
		return nil, "", errors.New("GUSTO_NODE_PRIVATE is this node's IPv4 address on the private network, which its servers bind to")
	}
	if cfg.nodesDir == "" {
		return nil, "", errors.New("GUSTO_NODES_DIR is the directory every node and gusto cloud see")
	}
	secret, err := nodeSecret("GUSTO_NODE_SECRET_FILE", cfg.secretFile)
	if err != nil {
		return nil, "", err
	}
	agent = &node.Agent{Name: cfg.name, Private: cfg.private, Secret: secret, Servers: &cloud.LocalNode{}}
	return agent, net.JoinHostPort(cfg.private, strconv.Itoa(cfg.port)), nil
}

// nodeSecret reads the secret gusto cloud and the nodes sign with from path, which the setting called name names. One too
// short to be a secret is refused, since everything a node will run is authorized by it.
func nodeSecret(name, path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New(name + " is the file with the secret gusto cloud and the nodes sign requests with")
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	secret := []byte(strings.TrimSpace(string(text)))
	if len(secret) < 32 {
		return nil, errors.New("the secret in " + path + " is too short to be one: 32 characters at least")
	}
	return secret, nil
}

// nodePoolSettings is the nodes gusto cloud may place organizations on.
type nodePoolSettings struct {
	dir, secretFile string
	goneMinutes     int
	local           bool
}

func (c *nodePoolSettings) declare(set *settings.Set) {
	set.String(&c.dir, "GUSTO_CLOUD_NODES_DIR", "", "the directory nodes register in; unset, this machine runs every organization's server")
	set.String(&c.secretFile, "GUSTO_CLOUD_NODE_SECRET_FILE", "", "the file with the secret gusto cloud and the nodes sign with")
	set.Int(&c.goneMinutes, "GUSTO_CLOUD_NODE_GONE_MINUTES", 30, 1, "how long a node may be silent before it is taken to be gone and its organizations are placed on another, in minutes")
	set.Bool(&c.local, "GUSTO_CLOUD_LOCAL_NODE", "this machine is a node too")
	renamedFromCell(set, "GUSTO_CLOUD_NODES_DIR", "GUSTO_CLOUD_NODE_SECRET_FILE", "GUSTO_CLOUD_NODE_GONE_MINUTES",
		"GUSTO_CLOUD_LOCAL_NODE")
}

// nodePool is the nodes gusto cloud may place organizations on, or nothing when there is one machine.
func nodePool(cfg nodePoolSettings, local *cloud.LocalNode) (*node.Pool, error) {
	if cfg.dir == "" {
		return nil, nil
	}
	secret, err := nodeSecret("GUSTO_CLOUD_NODE_SECRET_FILE", cfg.secretFile)
	if err != nil {
		return nil, err
	}
	pool := &node.Pool{Dir: cfg.dir, Secret: secret, MaxAge: 30 * time.Second, MinFreeMiB: 4096,
		GoneAfter: time.Duration(cfg.goneMinutes) * time.Minute}
	if cfg.local {
		pool.Local = local
	}
	return pool, nil
}

// beatForNodes says every few seconds that gusto cloud is there, which the nodes act on:
// a node with no gusto cloud for a long while has nobody to work for.
func beatForNodes(ctx context.Context, dir string) {
	for {
		if err := node.Beat(dir, time.Now()); err != nil {
			fail(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// removedScalerSettings refuses the settings gusto cloud started and stopped nodes on EC2 with, which it no longer does.
func removedScalerSettings(set *settings.Set) {
	for _, name := range []string{"GUSTO_CLOUD_NODE_LAUNCH", "GUSTO_CLOUD_NODE_PROJECT", "GUSTO_CLOUD_NODE_TEMPLATE", "GUSTO_CLOUD_NODE_USERDATA",
		"GUSTO_CLOUD_NODE_SETTINGS", "GUSTO_CLOUD_NODE_IMAGE_PARAMETER", "GUSTO_CLOUD_NODE_MAX", "GUSTO_CLOUD_NODE_IDLE_MINUTES",
		"GUSTO_CLOUD_NODE_PREWARMS", "GUSTO_CLOUD_NODE_KEEP"} {
		set.Removed(name, "start nodes yourself and set GUSTO_CLOUD_NODES_DIR, or run every server on this machine with GUSTO_CLOUD_SERVERS=shared")
	}
}

// renamedFromCell refuses each setting under the name it had when a node was called a cell, naming the new one.
func renamedFromCell(set *settings.Set, names ...string) {
	for _, name := range names {
		set.Renamed(name, strings.NewReplacer("NODES", "CELLS", "NODE", "CELL").Replace(name))
	}
}

func nodeCommandOf() *cmdline.Command {
	return &cmdline.Command{
		Name: "node", Group: groupRun, Summary: "Run a node for organizations' servers, for a gusto cloud",
		Description: "Runs the agent on a node that runs organizations' servers for a gusto cloud on another machine. It starts " +
			"and stops what gusto cloud asks, signed with the secret they share, and says every few seconds that it is there.",
		Env:     envNode(),
		SeeAlso: []string{"cloud"},
		Run:     none("node", nodeCommand),
	}
}
