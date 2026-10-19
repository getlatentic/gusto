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

	"github.com/getlatentic/gusto/internal/accesslog"
	"github.com/getlatentic/gusto/internal/cloud"
)

// cloudWorkspaces lists every workspace on this host.
func cloudWorkspaces() int {
	store, err := cloud.Open(setting("GUSTO_CLOUD_ROOT", "cloud"), cloud.Settings{})
	if err != nil {
		return fail(err)
	}
	if err := listComputers(os.Stdout, store); err != nil {
		return fail(err)
	}
	return 0
}

// cloudCommand runs gusto cloud: sign-up, sign-in and the organisations' records,
// and the servers it starts for each organisation and sends their people to. It runs
// no tasks itself.
func cloudCommand() int {
	front, controller, err := openCloud()
	if err != nil {
		return fail(err)
	}
	address := setting("GUSTO_CLOUD_ADDR", "127.0.0.1:8050")
	// There is no write timeout: an organisation's task stream stays open for as long as
	// the task runs, and gusto cloud carries it.
	server := &http.Server{
		Addr:              address,
		Handler:           accesslog.Keep(accesslog.Stderr())(front.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if dir := os.Getenv("GUSTO_CLOUD_CELLS_DIR"); dir != "" && controller != nil {
		go beatForCells(ctx, dir)
	}
	go front.RunDemos(ctx, 15*time.Second)
	go front.RunLeads(ctx, 30*time.Second)
	scaler, err := cellScaler(controller)
	if err != nil {
		return fail(err)
	}
	if scaler != nil {
		go scaler.Run(ctx, 10*time.Second)
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
	fmt.Printf("gusto cloud is listening on %s\n", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	front.Wait()
	<-controlled
	return 0
}

// openCloud reads gusto cloud's settings and opens what it keeps. Links
// that expired are swept when it starts and every hour after.
func openCloud() (*cloud.Server, *cloud.Controller, error) {
	address := setting("GUSTO_CLOUD_ADDR", "127.0.0.1:8050")
	public, err := url.Parse(setting("GUSTO_CLOUD_URL", "http://"+address))
	if err != nil || (public.Scheme != "http" && public.Scheme != "https") || public.Host == "" {
		return nil, nil, errors.New("GUSTO_CLOUD_URL is the address links in mail begin with, as https://cloud.example.com")
	}
	root := setting("GUSTO_CLOUD_ROOT", "cloud")
	limits := cloud.DefaultLimits()
	limits.Trial.Jobs = number("GUSTO_CLOUD_TRIAL_JOBS", limits.Trial.Jobs)
	limits.Trial.Concurrent = number("GUSTO_CLOUD_TRIAL_CONCURRENT", limits.Trial.Concurrent)
	limits.Verified.Jobs = number("GUSTO_CLOUD_JOBS", limits.Verified.Jobs)
	limits.Verified.Concurrent = number("GUSTO_CLOUD_CONCURRENT", limits.Verified.Concurrent)
	limits.Verified.Members = number("GUSTO_CLOUD_MEMBERS", limits.Verified.Members)
	limits.Verified.Computers = number("GUSTO_CLOUD_COMPUTERS", limits.Verified.Computers)
	store, err := cloud.Open(root, cloud.Settings{
		AllowFreeMail:  os.Getenv("GUSTO_CLOUD_FREE_MAIL") == "allow",
		RefusedDomains: lowered(values("GUSTO_CLOUD_REFUSED_DOMAINS")),
		PerDomain:      number("GUSTO_CLOUD_PER_DOMAIN", 3),
		Limits:         limits,
		Demo:           demoSettings(),
	})
	if err != nil {
		return nil, nil, err
	}
	sessions, err := cloud.OpenSessions(root, public.Scheme == "https")
	if err != nil {
		return nil, nil, err
	}
	mail, err := cloud.MailFrom(os.Getenv("GUSTO_CLOUD_MAIL"), filepath.Join(root, "outbox"), os.Getenv("GUSTO_CLOUD_MAIL_TOKEN"))
	if err != nil {
		return nil, nil, err
	}
	var controller *cloud.Controller
	if os.Getenv("GUSTO_CLOUD_SERVERS") != "off" {
		if controller, err = organisationServers(store, public, limits); err != nil {
			return nil, nil, err
		}
	}
	var lease *cloud.Lease
	if path := os.Getenv("GUSTO_CLOUD_LEASE_FILE"); path != "" {
		lease = cloud.OpenLease(path, time.Duration(number("GUSTO_CLOUD_LEASE_STEP_MINUTES", 30))*time.Minute)
	}
	front, err := cloud.New(cloud.Config{
		Lease: lease, Store: store, Mail: mail, Sessions: sessions, URL: public, Controller: controller,
		Domain:     setting("GUSTO_CLOUD_DOMAIN", "localhost"),
		TrustProxy: os.Getenv("GUSTO_CLOUD_TRUST_PROXY") == "1",
		Notify:     os.Getenv("GUSTO_CLOUD_DEMO_NOTIFY"),
		Leads:      &cloud.LeadHook{URL: os.Getenv("GUSTO_CLOUD_LEAD_HOOK_URL"), Secret: os.Getenv("GUSTO_CLOUD_LEAD_HOOK_SECRET"), Dir: filepath.Join(root, "leadhook")},
		Turnstile:  cloud.Turnstile{Site: os.Getenv("GUSTO_CLOUD_TURNSTILE_SITE"), Secret: os.Getenv("GUSTO_CLOUD_TURNSTILE_SECRET")},
		PerAddress: number("GUSTO_CLOUD_PER_ADDRESS", 0), PerMailbox: number("GUSTO_CLOUD_PER_MAILBOX", 0), InAll: number("GUSTO_CLOUD_IN_ALL", 0),
	})
	if err != nil {
		return nil, nil, err
	}
	go sweepLinks(store)
	return front, controller, nil
}

// organisationServers is what starts each organisation's gusto serve on this machine.
// GUSTO_CLOUD_ORG_ENV names a file of KEY=VALUE lines that every organisation's
// server is started with, which is where the operator's model credentials, models and
// sandbox settings go. Nothing else of gusto cloud's environment goes to them.
func organisationServers(store *cloud.Store, public *url.URL, limits cloud.Limits) (*cloud.Controller, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	env, err := operatorEnv(os.Getenv("GUSTO_CLOUD_ORG_ENV"))
	if err != nil {
		return nil, err
	}
	local := &cloud.LocalCell{}
	controller := &cloud.Controller{
		Store: store, Cell: local, Command: []string{exe, "serve"}, Env: env,
		FrontDoor: public.Scheme + "://" + public.Host, PortFrom: number("GUSTO_CLOUD_PORT_FROM", 9100),
		Wait: time.Duration(number("GUSTO_CLOUD_START_WAIT_SECONDS", 120)) * time.Second, Concurrent: limits.Verified.Concurrent,
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
			fmt.Fprintf(os.Stderr, "gusto cloud could not sweep expired links: %v\n", err)
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

// demoSettings make gusto cloud offer a demo when GUSTO_CLOUD_DEMO_MINUTES is set: the minutes a demo lasts
// from when it is ready, GUSTO_CLOUD_DEMO_EXTEND_MINUTES for its one extension (10), GUSTO_CLOUD_DEMO_MAX for the
// most at once (5), GUSTO_CLOUD_DEMO_PER_DAY for the most in a day (40), GUSTO_CLOUD_DEMO_PER_PERSON and GUSTO_CLOUD_DEMO_PER_COMPANY for the most one address (2) and one company (6) may start in twenty-four hours, GUSTO_CLOUD_DEMO_NOTIFY for an address that is mailed when a visitor starts a demo, GUSTO_CLOUD_DEMO_VIDEO for the video the landing page shows (an https address of a file or of a page to embed), and GUSTO_CLOUD_BOOK_URL for where a call is booked.
func demoSettings() cloud.DemoSettings {
	length := number("GUSTO_CLOUD_DEMO_MINUTES", 0)
	if length == 0 {
		return cloud.DemoSettings{}
	}
	return cloud.DemoSettings{
		Length:     time.Duration(length) * time.Minute,
		Extend:     time.Duration(number("GUSTO_CLOUD_DEMO_EXTEND_MINUTES", 10)) * time.Minute,
		Patience:   time.Duration(number("GUSTO_CLOUD_DEMO_PATIENCE_MINUTES", 15)) * time.Minute,
		MaxOrgs:    number("GUSTO_CLOUD_DEMO_MAX", 5),
		PerDay:     number("GUSTO_CLOUD_DEMO_PER_DAY", 40),
		PerPerson:  number("GUSTO_CLOUD_DEMO_PER_PERSON", 2),
		PerCompany: number("GUSTO_CLOUD_DEMO_PER_COMPANY", 6),
		BookURL:    os.Getenv("GUSTO_CLOUD_BOOK_URL"),
		VideoURL:   os.Getenv("GUSTO_CLOUD_DEMO_VIDEO"),
	}
}
