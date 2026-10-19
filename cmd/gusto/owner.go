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

	"github.com/getlatentic/gusto/internal/cell"
	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/owner"
)

// operatorCommand serves the page the operator of a gusto cloud manages it from. It listens on the loopback of the machine
// gusto cloud runs on, at GUSTO_OWNER_ADDR (127.0.0.1:8090), and refuses any other address, so that nothing gusto cloud's own
// listener or its web server serves can reach it. The operator opens a tunnel to it.
func operatorCommand() int {
	address := setting("GUSTO_OWNER_ADDR", "127.0.0.1:8090")
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fail(fmt.Errorf("GUSTO_OWNER_ADDR is host:port: %w", err))
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fail(errors.New("GUSTO_OWNER_ADDR is a loopback address, such as 127.0.0.1:8090, since the operator's page is not served to the network"))
	}
	store, err := cloud.Attach(setting("GUSTO_CLOUD_ROOT", "cloud"), cloud.Settings{Demo: demoSettings(), DatabaseURL: cloudDatabaseURL()})
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	var hosts owner.Hosts
	if os.Getenv("GUSTO_CLOUD_CELL_LAUNCH") == "ec2" {
		hosts = &cell.EC2{Project: os.Getenv("GUSTO_CLOUD_CELL_PROJECT")}
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
	fmt.Printf("the operator's page is on %s, which is this machine only\n", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	return 0
}
