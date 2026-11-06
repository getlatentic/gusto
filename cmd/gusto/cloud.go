package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/accesslog"
	"github.com/getlatentic/gusto/internal/cell"
	"github.com/getlatentic/gusto/internal/cloud"
)

// cloudWorkspaces lists every workspace on this host.
func cloudWorkspaces() int {
	var records cloudRecords
	if err := loadSettings(records.declare); err != nil {
		return refused(err)
	}
	store, err := cloud.Attach(records.root, records.accounts())
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	if err := listComputers(context.Background(), os.Stdout, store); err != nil {
		return fail(err)
	}
	return 0
}

// cloudCommand runs gusto cloud: sign-up, sign-in and the organizations' records,
// and the servers it starts for each organization and sends their people to. It runs
// no tasks itself.
func cloudCommand() int {
	cfg, err := loadCloud(os.Getenv)
	if err != nil {
		return refused(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	front, controller, err := openCloud(ctx, cfg)
	if err != nil {
		return fail(err)
	}
	defer front.Store.Close()
	scaler, err := cellScaler(cfg.scaler, controller)
	if err != nil {
		return fail(err)
	}
	door := &frontDoor{front: front, controller: controller, scaler: scaler}
	door.startBackground(ctx, cfg.cells.dir)
	controlled := door.leadInTurn(ctx, time.Duration(cfg.front.leaderSeconds)*time.Second)
	if err := serveCloud(ctx, cfg.front.addr, front); err != nil {
		return fail(err)
	}
	front.Wait()
	<-controlled
	return 0
}

// frontDoor is a running gusto cloud: its pages, the controller of the organizations' servers when it starts them, and
// the scaler of hosts when it starts those.
type frontDoor struct {
	front      *cloud.Server
	controller *cloud.Controller
	scaler     *cell.Scaler
}

// startBackground starts what every front door does, leader or not.
func (d *frontDoor) startBackground(ctx context.Context, cellsDir string) {
	if d.scaler != nil {
		d.controller.OnWaiting = d.scaler.Wake
		d.front.Prewarm = d.scaler.Prewarm
	}
	if cellsDir != "" && d.controller != nil {
		go beatForCells(ctx, cellsDir)
	}
	go d.front.RunLeads(ctx, 30*time.Second)
	if d.front.Seats != nil {
		go d.front.Seats.Run(ctx, time.Minute)
	}
}

// leadInTurn makes this front door one of those on the database that take turns to lead. The channel closes once it no
// longer leads and has stopped the servers it controlled.
func (d *frontDoor) leadInTurn(ctx context.Context, ttl time.Duration) <-chan struct{} {
	if d.controller != nil {
		d.controller.Follow()
	}
	leadership := &cloud.Leadership{Store: d.front.Store, Holder: holderName(), TTL: ttl, Every: ttl / 3}
	controlled := make(chan struct{})
	go func() {
		defer close(controlled)
		leadership.Run(ctx, d.lead)
		if d.controller != nil {
			d.controller.StopAll()
		}
	}()
	return controlled
}

// lead runs what one front door on the database does while its lease holds: the controller, the demo and quota sweeps and
// the scaler. The others serve pages and route from the database.
func (d *frontDoor) lead(ctx context.Context, epoch int64) {
	if d.controller != nil {
		d.controller.Lead(epoch)
		defer d.controller.Follow()
	}
	var wg sync.WaitGroup
	wg.Go(func() { d.front.RunDemos(ctx, 15*time.Second) })
	wg.Go(func() { d.front.Store.RunQuotas(ctx, 30*time.Second) })
	if d.scaler != nil {
		wg.Go(func() { d.scaler.Run(ctx, 10*time.Second) })
	}
	if d.controller != nil {
		wg.Go(func() { d.controller.Run(ctx, 5*time.Second) })
	}
	wg.Wait()
}

// serveCloud serves the front door's pages on address until ctx ends.
func serveCloud(ctx context.Context, address string, front *cloud.Server) error {
	// There is no write timeout: an organization's task stream stays open for as long as
	// the task runs, and gusto cloud carries it.
	server := &http.Server{
		Addr:              address,
		Handler:           accesslog.Keep(accesslog.Stderr())(front.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("gusto cloud is listening on %s, with its accounts in %s\n", address, front.Store.Database())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// holderName tells this front door apart from every other on the database: its host, its process and a random part, since
// a process number is used again after a restart.
func holderName() string {
	host, _ := os.Hostname()
	random := make([]byte, 4)
	_, _ = rand.Read(random)
	return fmt.Sprintf("%s/%d/%s", host, os.Getpid(), hex.EncodeToString(random))
}

// openCloud opens what gusto cloud keeps. Links that expired are swept when it starts and every hour after. A database that
// has no accounts yet takes in the files an earlier release kept under the root.
func openCloud(ctx context.Context, cfg cloudSettings) (*cloud.Server, *cloud.Controller, error) {
	public, err := cfg.front.public()
	if err != nil {
		return nil, nil, err
	}
	stripe, err := cfg.billing.stripe()
	if err != nil {
		return nil, nil, err
	}
	store, err := openAccounts(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	front, controller, err := frontOf(ctx, store, public, stripe, cfg)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	go sweepLinks(ctx, store)
	return front, controller, nil
}

// openAccounts opens the accounts and writes every organization's quota as its plan says.
func openAccounts(ctx context.Context, cfg cloudSettings) (*cloud.Store, error) {
	accounts := cfg.records.accounts()
	accounts.AllowFreeMail = cfg.signup.freeMail == "allow"
	accounts.RefusedDomains = lowered(cfg.signup.refusedDomains)
	accounts.PerDomain = cfg.signup.perDomain
	accounts.Limits = cfg.signup.limits()
	accounts.Demo = cfg.demo.forCloud()
	store, err := cloud.Open(cfg.records.root, accounts)
	if err != nil {
		return nil, err
	}
	if err := store.ApplyQuotas(ctx); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

// frontOf is the front door's pages on the accounts in store, and the controller of the organizations' servers when it
// starts them.
func frontOf(ctx context.Context, store *cloud.Store, public *url.URL, stripe *cloud.Stripe, cfg cloudSettings) (*cloud.Server, *cloud.Controller, error) {
	sessions, err := store.OpenSessions(ctx, public.Scheme == "https")
	if err != nil {
		return nil, nil, err
	}
	mail, err := cloud.MailFrom(cfg.front.mail, filepath.Join(cfg.records.root, "outbox"), cfg.front.mailToken)
	if err != nil {
		return nil, nil, err
	}
	controller, err := controllerOf(store, public, cfg)
	if err != nil {
		return nil, nil, err
	}
	f := cfg.front
	front, err := cloud.New(cloud.Config{
		Lease: f.lease(), Store: store, Mail: mail, Sessions: sessions, URL: public, Controller: controller,
		Domain: f.domain, TrustProxy: f.trustProxy, Notify: cfg.demo.notify,
		Leads:      &cloud.LeadHook{URL: f.leadHookURL, Secret: f.leadHookSecret, Dir: filepath.Join(cfg.records.root, "leadhook")},
		Turnstile:  cloud.Turnstile{Site: f.turnstileSite, Secret: f.turnstileSecret},
		PerAddress: f.perAddress, PerMailbox: f.perMailbox, InAll: f.inAll,
		Stripe: stripe,
	})
	if err != nil {
		return nil, nil, err
	}
	if stripe != nil {
		front.Seats = cloud.NewSeatSync(store, stripe)
	}
	return front, controller, nil
}

// controllerOf is what starts the organizations' servers, and nil when gusto cloud keeps the accounts only.
func controllerOf(store *cloud.Store, public *url.URL, cfg cloudSettings) (*cloud.Controller, error) {
	if cfg.servers.run == "off" {
		return nil, nil
	}
	controller, err := organizationServers(store, public, cfg)
	if err != nil {
		return nil, err
	}
	controller.OrgURL = func(slug string) string { return cloud.OrgAddress(public, cfg.front.domain, slug) }
	return controller, nil
}

// organizationServers is what starts each organization's gusto serve on this machine, or on the hosts when there are some.
// Every organization's server is started with the lines of GUSTO_CLOUD_ORG_ENV, and a demo organization's with those of
// GUSTO_CLOUD_DEMO_ENV on top. Nothing else of gusto cloud's environment goes to them.
func organizationServers(store *cloud.Store, public *url.URL, cfg cloudSettings) (*cloud.Controller, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	env, err := operatorEnv(cfg.servers.orgEnv)
	if err != nil {
		return nil, err
	}
	demoEnv, err := envLines(cfg.servers.demoEnv)
	if err != nil {
		return nil, err
	}
	indexDir := cfg.servers.indexDir
	if indexDir == "" {
		indexDir = filepath.Join(store.Root(), "index")
	}
	local := &cloud.LocalCell{}
	controller := &cloud.Controller{
		Store: store, Cell: local, Command: []string{exe, "serve"}, Env: env, DemoEnv: demoEnv,
		FrontDoor: public.Scheme + "://" + public.Host, PortFrom: cfg.servers.portFrom,
		Wait: time.Duration(cfg.servers.startWaitSeconds) * time.Second, Concurrent: cfg.signup.concurrent, IndexDir: indexDir,
	}
	pool, err := cellPool(cfg.cells, local)
	if err != nil || pool == nil {
		return controller, err
	}
	controller.Cells = pool
	// A host apart runs the servers from the shared files, as this release, and not the program it was made with.
	shared, err := cloud.ShareRelease(exe, filepath.Join(store.Root(), "releases"))
	if err != nil {
		return nil, fmt.Errorf("this release could not be put where the hosts run it from: %w", err)
	}
	controller.Command = []string{shared, "serve"}
	return controller, nil
}

// operatorEnv reads the settings every organization's server gets. A line is
// KEY=VALUE, and blank lines and lines that begin with # are skipped. PATH is
// this process's unless the file sets it, so a server finds git and openshell.
func operatorEnv(path string) ([]string, error) {
	lines, err := envLines(path)
	return append([]string{"PATH=" + os.Getenv("PATH")}, lines...), err
}

// envLines are the KEY=VALUE lines of a file of settings, without its blank lines and comments, and none when no file is named.
func envLines(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env []string
	for number, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "=") {
			return nil, fmt.Errorf("%s line %d is not KEY=VALUE", path, number+1)
		}
		env = append(env, line)
	}
	return env, nil
}

// sweepLinks removes expired links now and every hour, until ctx ends.
func sweepLinks(ctx context.Context, store *cloud.Store) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if _, err := store.Sweep(ctx); err != nil && ctx.Err() == nil {
			slog.Error("expired links could not be swept", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func lowered(members []string) []string {
	out := make([]string, len(members))
	for i, member := range members {
		out[i] = strings.ToLower(member)
	}
	return out
}
