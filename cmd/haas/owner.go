package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlatentic/haas/internal/cell"
	"github.com/getlatentic/haas/internal/cloud"
	"github.com/getlatentic/haas/internal/owner"
)

// ownerCommand serves the page the owner of a hosted haas manages it from. It listens on the loopback of the machine
// the front door runs on, at HAAS_OWNER_ADDR (127.0.0.1:8090), and refuses any other address, so that nothing the
// front door's own listener or its web server serves can reach it. The owner opens a tunnel to it.
func ownerCommand([]string) int {
	address := setting("HAAS_OWNER_ADDR", "127.0.0.1:8090")
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fail(fmt.Errorf("HAAS_OWNER_ADDR is host:port: %w", err))
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fail(errors.New("HAAS_OWNER_ADDR is a loopback address, such as 127.0.0.1:8090, since the owner's page is not served to the network"))
	}
	store, err := cloud.Attach(setting("HAAS_CLOUD_ROOT", "cloud"), cloud.Settings{Demo: demoSettings()})
	if err != nil {
		return fail(err)
	}
	var hosts owner.Hosts
	if os.Getenv("HAAS_CLOUD_CELL_LAUNCH") == "ec2" {
		hosts = &cell.EC2{Project: os.Getenv("HAAS_CLOUD_CELL_PROJECT")}
	}
	app, err := owner.New(store, hosts)
	if err != nil {
		return fail(err)
	}
	server := &http.Server{Addr: address, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("the owner's page is on %s, which is this machine only\n", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	return 0
}
