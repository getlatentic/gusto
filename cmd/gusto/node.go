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
	dir, secretFile         string
	minFreeMiB, goneMinutes int
	local                   bool
}

func (c *nodePoolSettings) declare(set *settings.Set) {
	set.String(&c.dir, "GUSTO_CLOUD_NODES_DIR", "", "the directory nodes register in; unset, this machine runs every organization's server")
	set.String(&c.secretFile, "GUSTO_CLOUD_NODE_SECRET_FILE", "", "the file with the secret gusto cloud and the nodes sign with")
	set.Int(&c.minFreeMiB, "GUSTO_CLOUD_NODE_MIN_FREE_MIB", 4096, 1, "the memory a node must have free for an organization to be placed on it, in MiB")
	set.Int(&c.goneMinutes, "GUSTO_CLOUD_NODE_GONE_MINUTES", 30, 1, "how long a node may be silent before it is taken to be gone and its organizations are placed on another, in minutes")
	set.Bool(&c.local, "GUSTO_CLOUD_LOCAL_NODE", "this machine is a node too")
	renamedFromCell(set, "GUSTO_CLOUD_NODES_DIR", "GUSTO_CLOUD_NODE_SECRET_FILE", "GUSTO_CLOUD_NODE_MIN_FREE_MIB",
		"GUSTO_CLOUD_NODE_GONE_MINUTES", "GUSTO_CLOUD_LOCAL_NODE")
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
	pool := &node.Pool{Dir: cfg.dir, Secret: secret, MaxAge: 30 * time.Second, MinFreeMiB: cfg.minFreeMiB,
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

// launchSettings is how nodes for organizations' servers are started, which gusto cloud and the operator's page both read.
type launchSettings struct{ launch, project string }

func (l *launchSettings) declare(set *settings.Set) {
	set.Choice(&l.launch, "GUSTO_CLOUD_NODE_LAUNCH", "", []string{"ec2"}, "how a node is started when an organization waits for room; unset, none is")
	set.String(&l.project, "GUSTO_CLOUD_NODE_PROJECT", "", "the project tag of the nodes' machines")
	renamedFromCell(set, "GUSTO_CLOUD_NODE_LAUNCH", "GUSTO_CLOUD_NODE_PROJECT")
}

// scalerSettings is how gusto cloud starts and stops nodes: what a node is made from, and how many may run.
type scalerSettings struct {
	launchSettings
	template, userData, imageParameter      string
	settings                                []string
	max, idleMinutes, bootMinutes, prewarms int
	keep                                    int
}

func (s *scalerSettings) declare(set *settings.Set) {
	s.launchSettings.declare(set)
	set.String(&s.template, "GUSTO_CLOUD_NODE_TEMPLATE", "", "the launch template a node's machine is made from")
	set.String(&s.userData, "GUSTO_CLOUD_NODE_USERDATA", "", "the start-up script a node's machine runs")
	set.List(&s.settings, "GUSTO_CLOUD_NODE_SETTINGS", "NAME=value pairs, comma-separated, that fill in the start-up script")
	set.String(&s.imageParameter, "GUSTO_CLOUD_NODE_IMAGE_PARAMETER", "", "the parameter the template reads its image from, so a kept node of an older image is replaced and not started again")
	set.Int(&s.max, "GUSTO_CLOUD_NODE_MAX", 3, 1, "the most nodes")
	set.Int(&s.idleMinutes, "GUSTO_CLOUD_NODE_IDLE_MINUTES", 10, 1, "how long a node may have nothing on it before it is stopped, in minutes")
	set.Int(&s.bootMinutes, "GUSTO_CLOUD_NODE_BOOT_MINUTES", 10, 1, "how long a node has to come up, in minutes")
	// A visitor who types in the demo form and never sends it costs a node until it is idle.
	set.Int(&s.prewarms, "GUSTO_CLOUD_NODE_PREWARMS", 20, 1, "the most nodes started in a day for a visitor still filling in the demo form")
	// A kept node is stopped when idle and started again, which is faster than a new one; its disk costs while it waits.
	set.Int(&s.keep, "GUSTO_CLOUD_NODE_KEEP", 0, 0, "how many idle nodes are stopped and kept, rather than removed")
	renamedFromCell(set, "GUSTO_CLOUD_NODE_TEMPLATE", "GUSTO_CLOUD_NODE_USERDATA", "GUSTO_CLOUD_NODE_SETTINGS",
		"GUSTO_CLOUD_NODE_IMAGE_PARAMETER", "GUSTO_CLOUD_NODE_MAX", "GUSTO_CLOUD_NODE_IDLE_MINUTES", "GUSTO_CLOUD_NODE_BOOT_MINUTES",
		"GUSTO_CLOUD_NODE_PREWARMS", "GUSTO_CLOUD_NODE_KEEP")
}

// renamedFromCell refuses each setting under the name it had when a node was called a cell, naming the new one.
func renamedFromCell(set *settings.Set, names ...string) {
	for _, name := range names {
		set.Renamed(name, strings.NewReplacer("NODES", "CELLS", "NODE", "CELL").Replace(name))
	}
}

// nodeScaler starts a node when an organization waits for room and stops one that has had nothing on it, so no node runs
// while nobody is using one. It is nil unless nodes are started on EC2.
func nodeScaler(cfg scalerSettings, controller *cloud.Controller) (*node.Scaler, error) {
	if cfg.launch != "ec2" || controller == nil {
		return nil, nil
	}
	pool, ok := controller.Nodes.(*node.Pool)
	if !ok {
		return nil, errors.New("GUSTO_CLOUD_NODE_LAUNCH needs GUSTO_CLOUD_NODES_DIR, which says where nodes register")
	}
	if cfg.template == "" || cfg.userData == "" {
		return nil, errors.New("GUSTO_CLOUD_NODE_TEMPLATE and GUSTO_CLOUD_NODE_USERDATA say what a node's machine is made from")
	}
	filled, err := scriptSettings(cfg.settings)
	if err != nil {
		return nil, err
	}
	return &node.Scaler{
		Pool: pool, Waiting: controller.Waiting, Busy: controller.Busy,
		Launcher: &node.EC2{Template: cfg.template, UserData: cfg.userData, Settings: filled, Project: cfg.project, ImageParameter: cfg.imageParameter},
		Max:      cfg.max,
		Idle:     time.Duration(cfg.idleMinutes) * time.Minute,
		Boot:     time.Duration(cfg.bootMinutes) * time.Minute,
		Prewarms: cfg.prewarms,
		Keep:     cfg.keep,
	}, nil
}

// scriptSettings is the NAME=value pairs that fill in a node's start-up script.
func scriptSettings(pairs []string) (map[string]string, error) {
	filled := map[string]string{}
	for _, pair := range pairs {
		name, value, found := strings.Cut(pair, "=")
		if !found || name == "" {
			return nil, errors.New("GUSTO_CLOUD_NODE_SETTINGS is NAME=value pairs separated by commas")
		}
		filled[name] = value
	}
	return filled, nil
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
