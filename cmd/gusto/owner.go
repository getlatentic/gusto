package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/cell"
	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/owner"
	"github.com/getlatentic/gusto/internal/settings"
)

// operatorSettings is what gusto operator reads: where it listens, the accounts, and how hosts are started.
type operatorSettings struct {
	addr    string
	records cloudRecords
	demo    demoSettings
	hosts   launchSettings
}

func (o *operatorSettings) declare(set *settings.Set) {
	set.String(&o.addr, "GUSTO_OWNER_ADDR", "127.0.0.1:8090", "where to listen; a loopback address")
	o.records.declare(set)
	o.demo.declare(set)
	o.hosts.declare(set)
}

// operatorCommand serves the page the operator of a gusto cloud manages it from. It listens on the loopback of the machine
// gusto cloud runs on and refuses any other address, so that nothing gusto cloud's own listener or its web server serves can
// reach it. The operator opens a tunnel to it.
func operatorCommand() int {
	var cfg operatorSettings
	if err := loadSettings(cfg.declare); err != nil {
		return refused(err)
	}
	address := cfg.addr
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return refused(fmt.Errorf("GUSTO_OWNER_ADDR is host:port: %w", err))
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return refused(errors.New("GUSTO_OWNER_ADDR is a loopback address, such as 127.0.0.1:8090, since the operator's page is not served to the network"))
	}
	accounts := cfg.records.accounts()
	accounts.Demo = cfg.demo.forCloud()
	store, err := cloud.Attach(cfg.records.root, accounts)
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	var hosts owner.Hosts
	if cfg.hosts.launch == "ec2" {
		hosts = &cell.EC2{Project: cfg.hosts.project}
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
