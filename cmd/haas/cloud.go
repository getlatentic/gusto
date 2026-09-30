package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/haas/internal/accesslog"
	"github.com/getlatentic/haas/internal/cloud"
)

// cloudCommand runs the front door of a hosted haas: sign-up, sign-in and the
// organisations' records, and the servers it starts for each organisation and sends
// their people to. It runs no jobs itself.
func cloudCommand(args []string) int {
	if len(args) > 0 && args[0] == "computers" {
		store, err := cloud.Open(setting("HAAS_CLOUD_ROOT", "cloud"), cloud.Settings{})
		if err != nil {
			return fail(err)
		}
		if err := listComputers(os.Stdout, store); err != nil {
			return fail(err)
		}
		return 0
	}
	front, controller, err := openCloud()
	if err != nil {
		return fail(err)
	}
	address := setting("HAAS_CLOUD_ADDR", "127.0.0.1:8050")
	// There is no write timeout: an organisation's job stream stays open for as long as
	// the job runs, and the front door carries it.
	server := &http.Server{
		Addr:              address,
		Handler:           accesslog.Keep(accesslog.Stderr())(front.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if dir := os.Getenv("HAAS_CLOUD_CELLS_DIR"); dir != "" && controller != nil {
		go beatForCells(ctx, dir)
	}
	controlled := make(chan struct{})
	go func() {
		defer close(controlled)
		if controller != nil {
			controller.Run(ctx, 5*time.Second)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("the front door is listening on %s\n", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	front.Wait()
	<-controlled
	return 0
}

// openCloud reads the front door's settings and opens what it keeps. Links
// that expired are swept when it starts and every hour after.
func openCloud() (*cloud.Server, *cloud.Controller, error) {
	address := setting("HAAS_CLOUD_ADDR", "127.0.0.1:8050")
	public, err := url.Parse(setting("HAAS_CLOUD_URL", "http://"+address))
	if err != nil || (public.Scheme != "http" && public.Scheme != "https") || public.Host == "" {
		return nil, nil, errors.New("HAAS_CLOUD_URL is the address links in mail begin with, as https://cloud.example.com")
	}
	root := setting("HAAS_CLOUD_ROOT", "cloud")
	limits := cloud.DefaultLimits()
	limits.Trial.Jobs = number("HAAS_CLOUD_TRIAL_JOBS", limits.Trial.Jobs)
	limits.Trial.Concurrent = number("HAAS_CLOUD_TRIAL_CONCURRENT", limits.Trial.Concurrent)
	limits.Verified.Jobs = number("HAAS_CLOUD_JOBS", limits.Verified.Jobs)
	limits.Verified.Concurrent = number("HAAS_CLOUD_CONCURRENT", limits.Verified.Concurrent)
	limits.Verified.Members = number("HAAS_CLOUD_MEMBERS", limits.Verified.Members)
	limits.Verified.Computers = number("HAAS_CLOUD_COMPUTERS", limits.Verified.Computers)
	store, err := cloud.Open(root, cloud.Settings{
		AllowFreeMail:  os.Getenv("HAAS_CLOUD_FREE_MAIL") == "allow",
		RefusedDomains: lowered(values("HAAS_CLOUD_REFUSED_DOMAINS")),
		PerDomain:      number("HAAS_CLOUD_PER_DOMAIN", 3),
		Limits:         limits,
	})
	if err != nil {
		return nil, nil, err
	}
	sessions, err := cloud.OpenSessions(root, public.Scheme == "https")
	if err != nil {
		return nil, nil, err
	}
	mail, err := cloud.MailFrom(os.Getenv("HAAS_CLOUD_MAIL"), filepath.Join(root, "outbox"), os.Getenv("HAAS_CLOUD_MAIL_TOKEN"))
	if err != nil {
		return nil, nil, err
	}
	var controller *cloud.Controller
	if os.Getenv("HAAS_CLOUD_SERVERS") != "off" {
		if controller, err = organisationServers(store, public, limits); err != nil {
			return nil, nil, err
		}
	}
	var lease *cloud.Lease
	if path := os.Getenv("HAAS_CLOUD_LEASE_FILE"); path != "" {
		lease = cloud.OpenLease(path, time.Duration(number("HAAS_CLOUD_LEASE_STEP_MINUTES", 30))*time.Minute)
	}
	front, err := cloud.New(cloud.Config{
		Lease: lease, Store: store, Mail: mail, Sessions: sessions, URL: public, Controller: controller,
		Domain:     setting("HAAS_CLOUD_DOMAIN", "localhost"),
		TrustProxy: os.Getenv("HAAS_CLOUD_TRUST_PROXY") == "1",
		PerAddress: number("HAAS_CLOUD_PER_ADDRESS", 0), PerMailbox: number("HAAS_CLOUD_PER_MAILBOX", 0), InAll: number("HAAS_CLOUD_IN_ALL", 0),
	})
	if err != nil {
		return nil, nil, err
	}
	go sweepLinks(store)
	return front, controller, nil
}

// organisationServers is what starts each organisation's haas serve on this machine.
// HAAS_CLOUD_ORG_ENV names a file of KEY=VALUE lines that every organisation's
// server is started with, which is where the operator's model credentials, models and
// sandbox settings go. Nothing else of the front door's environment goes to them.
func organisationServers(store *cloud.Store, public *url.URL, limits cloud.Limits) (*cloud.Controller, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	env, err := operatorEnv(os.Getenv("HAAS_CLOUD_ORG_ENV"))
	if err != nil {
		return nil, err
	}
	local := &cloud.LocalCell{}
	controller := &cloud.Controller{
		Store: store, Cell: local, Command: []string{exe, "serve"}, Env: env,
		FrontDoor: public.Scheme + "://" + public.Host, PortFrom: number("HAAS_CLOUD_PORT_FROM", 9100),
		Wait: time.Duration(number("HAAS_CLOUD_START_WAIT_SECONDS", 120)) * time.Second, Concurrent: limits.Verified.Concurrent,
	}
	pool, err := cellPool(local)
	if err != nil {
		return nil, err
	}
	if pool != nil {
		controller.Cells = pool
	}
	return controller, nil
}

// operatorEnv reads the settings every organisation's server gets. A line is
// KEY=VALUE, and blank lines and lines that begin with # are skipped. PATH is
// this process's unless the file sets it, so a server finds git and openshell.
func operatorEnv(path string) ([]string, error) {
	env := []string{"PATH=" + os.Getenv("PATH")}
	if path == "" {
		return env, nil
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
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

func sweepLinks(store *cloud.Store) {
	for {
		if _, err := store.Sweep(); err != nil {
			fmt.Fprintf(os.Stderr, "the front door could not sweep expired links: %v\n", err)
		}
		time.Sleep(time.Hour)
	}
}

func lowered(members []string) []string {
	out := make([]string, len(members))
	for i, member := range members {
		out[i] = strings.ToLower(member)
	}
	return out
}
