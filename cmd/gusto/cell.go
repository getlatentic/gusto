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

	"github.com/getlatentic/gusto/internal/cell"
	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/settings"
)

// hostSettings is what gusto host reads: what this host is called, where it listens and how it knows gusto cloud.
type hostSettings struct {
	name, private, secretFile, cellsDir string
	port                                int
}

func (h *hostSettings) declare(set *settings.Set) {
	set.String(&h.name, "GUSTO_CELL_NAME", "", "what this host is called: lower case letters, digits and hyphens, up to 31")
	set.String(&h.private, "GUSTO_CELL_PRIVATE", "", "this host's IPv4 address on the private network, which its servers bind to")
	set.Int(&h.port, "GUSTO_CELL_PORT", 8060, 1, "the port to listen on")
	set.String(&h.secretFile, "GUSTO_CELL_SECRET_FILE", "", "the file with the secret gusto cloud and the hosts sign with")
	set.String(&h.cellsDir, "GUSTO_CELLS_DIR", "", "the directory every host and gusto cloud see")
}

// hostCommand runs the agent on a host that runs organizations' servers for a gusto cloud on another
// host. It starts and stops what gusto cloud asks, signed with the secret they share, and says every few
// seconds that it is there.
func hostCommand() int {
	var cfg hostSettings
	if err := loadSettings(cfg.declare); err != nil {
		return refused(err)
	}
	agent, listen, err := cellOf(cfg)
	if err != nil {
		return refused(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := agent.Run(ctx, listen, cfg.cellsDir, 10*time.Second); err != nil {
		return fail(err)
	}
	return 0
}

// cellOf is the agent a host runs, and the address it listens on, or why the host's settings cannot make one.
func cellOf(cfg hostSettings) (agent *cell.Agent, listen string, err error) {
	if !cell.ValidName(cfg.name) {
		return nil, "", errors.New("GUSTO_CELL_NAME is what this host is called: lower case letters, digits and hyphens, up to 31")
	}
	if ip := net.ParseIP(cfg.private); ip == nil || ip.To4() == nil {
		return nil, "", errors.New("GUSTO_CELL_PRIVATE is this host's IPv4 address on the private network, which its servers bind to")
	}
	if cfg.cellsDir == "" {
		return nil, "", errors.New("GUSTO_CELLS_DIR is the directory every host and gusto cloud see")
	}
	secret, err := cellSecret("GUSTO_CELL_SECRET_FILE", cfg.secretFile)
	if err != nil {
		return nil, "", err
	}
	agent = &cell.Agent{Name: cfg.name, Private: cfg.private, Secret: secret, Servers: &cloud.LocalCell{}}
	return agent, net.JoinHostPort(cfg.private, strconv.Itoa(cfg.port)), nil
}

// cellSecret reads the secret gusto cloud and the hosts sign with from path, which the setting called name names. One too
// short to be a secret is refused, since everything a host will run is authorized by it.
func cellSecret(name, path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New(name + " is the file with the secret gusto cloud and the hosts sign requests with")
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

// cellPoolSettings is the hosts gusto cloud may place organizations on.
type cellPoolSettings struct {
	dir, secretFile         string
	minFreeMiB, goneMinutes int
	local                   bool
}

func (c *cellPoolSettings) declare(set *settings.Set) {
	set.String(&c.dir, "GUSTO_CLOUD_CELLS_DIR", "", "the directory hosts register in; unset, this machine runs every organization's server")
	set.String(&c.secretFile, "GUSTO_CLOUD_CELL_SECRET_FILE", "", "the file with the secret gusto cloud and the hosts sign with")
	set.Int(&c.minFreeMiB, "GUSTO_CLOUD_CELL_MIN_FREE_MIB", 4096, 1, "the memory a host must have free for an organization to be placed on it, in MiB")
	set.Int(&c.goneMinutes, "GUSTO_CLOUD_CELL_GONE_MINUTES", 30, 1, "how long a host may be silent before it is taken to be gone and its organizations are placed on another, in minutes")
	set.Bool(&c.local, "GUSTO_CLOUD_LOCAL_CELL", "this machine is a host too")
}

// cellPool is the hosts gusto cloud may place organizations on, or nothing when there is one machine.
func cellPool(cfg cellPoolSettings, local *cloud.LocalCell) (*cell.Pool, error) {
	if cfg.dir == "" {
		return nil, nil
	}
	secret, err := cellSecret("GUSTO_CLOUD_CELL_SECRET_FILE", cfg.secretFile)
	if err != nil {
		return nil, err
	}
	pool := &cell.Pool{Dir: cfg.dir, Secret: secret, MaxAge: 30 * time.Second, MinFreeMiB: cfg.minFreeMiB,
		GoneAfter: time.Duration(cfg.goneMinutes) * time.Minute}
	if cfg.local {
		pool.Local = local
	}
	return pool, nil
}

// beatForCells says every few seconds that gusto cloud is there, which the hosts act on:
// a host with no gusto cloud for a long while has nobody to work for.
func beatForCells(ctx context.Context, dir string) {
	for {
		if err := cell.Beat(dir, time.Now()); err != nil {
			fail(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// launchSettings is how hosts for organizations' servers are started, which gusto cloud and the operator's page both read.
type launchSettings struct{ launch, project string }

func (l *launchSettings) declare(set *settings.Set) {
	set.Choice(&l.launch, "GUSTO_CLOUD_CELL_LAUNCH", "", []string{"ec2"}, "how a host is started when an organization waits for room; unset, none is")
	set.String(&l.project, "GUSTO_CLOUD_CELL_PROJECT", "", "the project tag of the hosts")
}

// scalerSettings is how gusto cloud starts and stops hosts: what a host is made from, and how many may run.
type scalerSettings struct {
	launchSettings
	template, userData, imageParameter      string
	settings                                []string
	max, idleMinutes, bootMinutes, prewarms int
	keep                                    int
}

func (s *scalerSettings) declare(set *settings.Set) {
	s.launchSettings.declare(set)
	set.String(&s.template, "GUSTO_CLOUD_CELL_TEMPLATE", "", "the launch template a host is made from")
	set.String(&s.userData, "GUSTO_CLOUD_CELL_USERDATA", "", "the start-up script a host runs")
	set.List(&s.settings, "GUSTO_CLOUD_CELL_SETTINGS", "NAME=value pairs, comma-separated, that fill in the start-up script")
	set.String(&s.imageParameter, "GUSTO_CLOUD_CELL_IMAGE_PARAMETER", "", "the parameter the template reads its image from, so a kept host of an older image is replaced and not started again")
	set.Int(&s.max, "GUSTO_CLOUD_CELL_MAX", 3, 1, "the most hosts")
	set.Int(&s.idleMinutes, "GUSTO_CLOUD_CELL_IDLE_MINUTES", 10, 1, "how long a host may have nothing on it before it is stopped, in minutes")
	set.Int(&s.bootMinutes, "GUSTO_CLOUD_CELL_BOOT_MINUTES", 10, 1, "how long a host has to come up, in minutes")
	// A visitor who types in the demo form and never sends it costs a cell until it is idle.
	set.Int(&s.prewarms, "GUSTO_CLOUD_CELL_PREWARMS", 20, 1, "the most hosts started in a day for a visitor still filling in the demo form")
	// A kept host is stopped when idle and started again, which is faster than a new one; its disk costs while it waits.
	set.Int(&s.keep, "GUSTO_CLOUD_CELL_KEEP", 0, 0, "how many idle hosts are stopped and kept, rather than removed")
}

// cellScaler starts a host when an organization waits for room and stops one that has had nothing on it, so no host runs
// while nobody is using one. It is nil unless hosts are started on EC2.
func cellScaler(cfg scalerSettings, controller *cloud.Controller) (*cell.Scaler, error) {
	if cfg.launch != "ec2" || controller == nil {
		return nil, nil
	}
	pool, ok := controller.Cells.(*cell.Pool)
	if !ok {
		return nil, errors.New("GUSTO_CLOUD_CELL_LAUNCH needs GUSTO_CLOUD_CELLS_DIR, which says where hosts register")
	}
	if cfg.template == "" || cfg.userData == "" {
		return nil, errors.New("GUSTO_CLOUD_CELL_TEMPLATE and GUSTO_CLOUD_CELL_USERDATA say what a host is made from")
	}
	filled, err := scriptSettings(cfg.settings)
	if err != nil {
		return nil, err
	}
	return &cell.Scaler{
		Pool: pool, Waiting: controller.Waiting, Busy: controller.Busy,
		Launcher: &cell.EC2{Template: cfg.template, UserData: cfg.userData, Settings: filled, Project: cfg.project, ImageParameter: cfg.imageParameter},
		Max:      cfg.max,
		Idle:     time.Duration(cfg.idleMinutes) * time.Minute,
		Boot:     time.Duration(cfg.bootMinutes) * time.Minute,
		Prewarms: cfg.prewarms,
		Keep:     cfg.keep,
	}, nil
}

// scriptSettings is the NAME=value pairs that fill in a host's start-up script.
func scriptSettings(pairs []string) (map[string]string, error) {
	filled := map[string]string{}
	for _, pair := range pairs {
		name, value, found := strings.Cut(pair, "=")
		if !found || name == "" {
			return nil, errors.New("GUSTO_CLOUD_CELL_SETTINGS is NAME=value pairs separated by commas")
		}
		filled[name] = value
	}
	return filled, nil
}
