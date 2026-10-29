package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/cell"
	"github.com/getlatentic/gusto/internal/cloud"
)

// hostCommand runs the agent on a host that runs organizations' servers for a gusto cloud on another
// host. It starts and stops what gusto cloud asks, signed with the secret they share, and says every few
// seconds that it is there. Its settings are GUSTO_CELL_NAME, GUSTO_CELL_PRIVATE (the address on the
// private network its servers bind to), GUSTO_CELL_PORT (8060), GUSTO_CELL_SECRET_FILE and GUSTO_CELLS_DIR,
// the directory every host and gusto cloud see.
func hostCommand() int {
	agent, listen, dir, err := cellFromEnv()
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := agent.Run(ctx, listen, dir, 10*time.Second); err != nil {
		return fail(err)
	}
	return 0
}

func cellFromEnv() (agent *cell.Agent, listen, dir string, err error) {
	name, private := os.Getenv("GUSTO_CELL_NAME"), os.Getenv("GUSTO_CELL_PRIVATE")
	if !cell.ValidName(name) {
		return nil, "", "", errors.New("GUSTO_CELL_NAME is what this host is called: lower case letters, digits and hyphens, up to 31")
	}
	if ip := net.ParseIP(private); ip == nil || ip.To4() == nil {
		return nil, "", "", errors.New("GUSTO_CELL_PRIVATE is this host's IPv4 address on the private network, which its servers bind to")
	}
	if dir = os.Getenv("GUSTO_CELLS_DIR"); dir == "" {
		return nil, "", "", errors.New("GUSTO_CELLS_DIR is the directory every host and gusto cloud see")
	}
	secret, err := cellSecret("GUSTO_CELL_SECRET_FILE")
	if err != nil {
		return nil, "", "", err
	}
	agent = &cell.Agent{Name: name, Private: private, Secret: secret, Servers: &cloud.LocalCell{}}
	return agent, net.JoinHostPort(private, setting("GUSTO_CELL_PORT", "8060")), dir, nil
}

// cellSecret reads the secret gusto cloud and the hosts sign with. One too short to be a secret is refused,
// since everything a host will run is authorized by it.
func cellSecret(setting string) ([]byte, error) {
	path := os.Getenv(setting)
	if path == "" {
		return nil, errors.New(setting + " is the file with the secret gusto cloud and the hosts sign requests with")
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

// cellPool is the hosts gusto cloud may place organizations on, or nothing when there is one machine.
// GUSTO_CLOUD_CELLS_DIR is the directory hosts register in, and GUSTO_CLOUD_CELL_SECRET_FILE the secret. This
// machine is a host too when GUSTO_CLOUD_LOCAL_CELL is on. A host silent for GUSTO_CLOUD_CELL_GONE_MINUTES (30)
// is taken to be gone, and the organizations on it are placed on another.
func cellPool(local *cloud.LocalCell) (*cell.Pool, error) {
	dir := os.Getenv("GUSTO_CLOUD_CELLS_DIR")
	if dir == "" {
		return nil, nil
	}
	secret, err := cellSecret("GUSTO_CLOUD_CELL_SECRET_FILE")
	if err != nil {
		return nil, err
	}
	pool := &cell.Pool{Dir: dir, Secret: secret, MaxAge: 30 * time.Second, MinFreeMiB: number("GUSTO_CLOUD_CELL_MIN_FREE_MIB", 4096),
		GoneAfter: time.Duration(number("GUSTO_CLOUD_CELL_GONE_MINUTES", 30)) * time.Minute}
	if os.Getenv("GUSTO_CLOUD_LOCAL_CELL") == "on" {
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

// cellScaler starts a host when an organization waits for room and stops one that has had nothing on it, so
// no host runs while nobody is using one. It is on when GUSTO_CLOUD_CELL_LAUNCH is ec2, and then needs
// GUSTO_CLOUD_CELL_TEMPLATE (the launch template), GUSTO_CLOUD_CELL_USERDATA (the start-up script) and
// GUSTO_CLOUD_CELL_SETTINGS, NAME=value pairs separated by commas that fill in the script. GUSTO_CLOUD_CELL_PROJECT
// is the project tag of the hosts (gusto-poc), and GUSTO_CLOUD_CELL_IMAGE_PARAMETER the parameter the template reads its
// image from, so a kept host of an older image is replaced and not started again. GUSTO_CLOUD_CELL_MAX (3) is
// the most hosts, GUSTO_CLOUD_CELL_IDLE_MINUTES (10) how long a host may have nothing on it, and
// GUSTO_CLOUD_CELL_BOOT_MINUTES (10) how long a host has to come up.
func cellScaler(controller *cloud.Controller) (*cell.Scaler, error) {
	if os.Getenv("GUSTO_CLOUD_CELL_LAUNCH") != "ec2" || controller == nil {
		return nil, nil
	}
	pool, ok := controller.Cells.(*cell.Pool)
	if !ok {
		return nil, errors.New("GUSTO_CLOUD_CELL_LAUNCH needs GUSTO_CLOUD_CELLS_DIR, which says where hosts register")
	}
	template, script := os.Getenv("GUSTO_CLOUD_CELL_TEMPLATE"), os.Getenv("GUSTO_CLOUD_CELL_USERDATA")
	if template == "" || script == "" {
		return nil, errors.New("GUSTO_CLOUD_CELL_TEMPLATE and GUSTO_CLOUD_CELL_USERDATA say what a host is made from")
	}
	settings := map[string]string{}
	for _, pair := range values("GUSTO_CLOUD_CELL_SETTINGS") {
		name, value, found := strings.Cut(pair, "=")
		if !found || name == "" {
			return nil, errors.New("GUSTO_CLOUD_CELL_SETTINGS is NAME=value pairs separated by commas")
		}
		settings[name] = value
	}
	return &cell.Scaler{
		Pool: pool, Launcher: &cell.EC2{Template: template, UserData: script, Settings: settings, Project: os.Getenv("GUSTO_CLOUD_CELL_PROJECT"),
			ImageParameter: os.Getenv("GUSTO_CLOUD_CELL_IMAGE_PARAMETER")}, Waiting: controller.Waiting,
		Max:  number("GUSTO_CLOUD_CELL_MAX", 3),
		Idle: time.Duration(number("GUSTO_CLOUD_CELL_IDLE_MINUTES", 10)) * time.Minute,
		Boot: time.Duration(number("GUSTO_CLOUD_CELL_BOOT_MINUTES", 10)) * time.Minute,
		// A visitor who types in the demo form and never sends it costs a cell until it is idle; this many a day.
		Prewarms: number("GUSTO_CLOUD_CELL_PREWARMS", 20),
		// A kept host is stopped when idle and started again, which is faster than a new one; its disk costs while it waits.
		Keep: number("GUSTO_CLOUD_CELL_KEEP", 0),
	}, nil
}
