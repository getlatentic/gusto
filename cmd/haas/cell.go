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

	"github.com/getlatentic/haas/internal/cell"
	"github.com/getlatentic/haas/internal/cloud"
)

// cellCommand runs the agent on a host that runs organisations' servers for a front door on another
// host. It starts and stops what the front door asks, signed with the secret they share, and says every few
// seconds that it is there. Its settings are HAAS_CELL_NAME, HAAS_CELL_PRIVATE (the address on the
// private network its servers bind to), HAAS_CELL_PORT (8060), HAAS_CELL_SECRET_FILE and HAAS_CELLS_DIR,
// the directory every cell and the front door see.
func cellCommand(args []string) int {
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
	name, private := os.Getenv("HAAS_CELL_NAME"), os.Getenv("HAAS_CELL_PRIVATE")
	if !cell.ValidName(name) {
		return nil, "", "", errors.New("HAAS_CELL_NAME is what this cell is called: lower case letters, digits and hyphens, up to 31")
	}
	if ip := net.ParseIP(private); ip == nil || ip.To4() == nil {
		return nil, "", "", errors.New("HAAS_CELL_PRIVATE is this host's IPv4 address on the private network, which its servers bind to")
	}
	if dir = os.Getenv("HAAS_CELLS_DIR"); dir == "" {
		return nil, "", "", errors.New("HAAS_CELLS_DIR is the directory every cell and the front door see")
	}
	secret, err := cellSecret("HAAS_CELL_SECRET_FILE")
	if err != nil {
		return nil, "", "", err
	}
	agent = &cell.Agent{Name: name, Private: private, Secret: secret, Servers: &cloud.LocalCell{}}
	return agent, net.JoinHostPort(private, setting("HAAS_CELL_PORT", "8060")), dir, nil
}

// cellSecret reads the secret the front door and the cells sign with. One too short to be a secret is refused,
// since everything a cell will run is authorised by it.
func cellSecret(setting string) ([]byte, error) {
	path := os.Getenv(setting)
	if path == "" {
		return nil, errors.New(setting + " is the file with the secret the front door and the cells sign requests with")
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

// cellPool is the cells the front door may place organisations on, or nothing when there is one machine.
// HAAS_CLOUD_CELLS_DIR is the directory cells register in, and HAAS_CLOUD_CELL_SECRET_FILE the secret. This
// machine is a cell too when HAAS_CLOUD_LOCAL_CELL is on.
func cellPool(local *cloud.LocalCell) (*cell.Pool, error) {
	dir := os.Getenv("HAAS_CLOUD_CELLS_DIR")
	if dir == "" {
		return nil, nil
	}
	secret, err := cellSecret("HAAS_CLOUD_CELL_SECRET_FILE")
	if err != nil {
		return nil, err
	}
	pool := &cell.Pool{Dir: dir, Secret: secret, MaxAge: 30 * time.Second, MinFreeMiB: number("HAAS_CLOUD_CELL_MIN_FREE_MIB", 4096)}
	if os.Getenv("HAAS_CLOUD_LOCAL_CELL") == "on" {
		pool.Local = local
	}
	return pool, nil
}

// beatForCells says every few seconds that the front door is there, which the hosts of the cells act on:
// a cell with no front door for a long while has nobody to work for.
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
