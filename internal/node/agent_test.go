package node

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// freePort is a port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// A node that goes stops its servers together, each given its grace, and takes its registration away, inside the time its
// service has to stop.
func TestAHostThatGoesStopsItsServersTogetherAndTakesItsRegistrationAway(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a server is asked to end by a signal Windows does not send")
	}
	dir := filepath.Join(t.TempDir(), "cells")
	servers := &cloud.LocalNode{}
	t.Cleanup(func() {
		for _, slug := range servers.Running() {
			servers.Stop(slug, time.Second)
		}
	})
	for _, slug := range []string{"acme-com", "beta-com", "gamma-com"} {
		spec := fakeOrg(t, slug, freePort(t))
		spec.Env = append(spec.Env, "CELL_IGNORE_TERM=1")
		if err := servers.Start(spec); err != nil {
			t.Fatal(err)
		}
	}
	agent := &Agent{Name: "cell-0aaa1111", Private: "127.0.0.1", Secret: secret, Servers: servers, Grace: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- agent.Run(ctx, "127.0.0.1:0", dir, time.Hour) }()
	waitFor(t, "the host registered", func() bool { _, err := os.Stat(file(dir, agent.Name)); return err == nil })
	began := time.Now()
	cancel()
	if err := <-ran; err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took > 2500*time.Millisecond {
		t.Errorf("three servers that do not end when asked took %s to stop, one after another", took.Round(time.Millisecond))
	}
	if _, err := os.Stat(file(dir, agent.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the registration is still there: %v", err)
	}
}
