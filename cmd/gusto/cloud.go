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

// cloudDatabaseURL is where the accounts are: empty is SQLite in cloud.db under the root.
func cloudDatabaseURL() string { return strings.TrimSpace(os.Getenv("GUSTO_DATABASE_URL")) }

// cloudWorkspaces lists every workspace on this host.
func cloudWorkspaces() int {
	store, err := cloud.Attach(setting("GUSTO_CLOUD_ROOT", "cloud"), cloud.Settings{DatabaseURL: cloudDatabaseURL()})
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	if err := listComputers(os.Stdout, store); err != nil {
		return fail(err)
	}
	return 0
}

// cloudCommand runs gusto cloud: sign-up, sign-in and the organizations' records,
// and the servers it starts for each organization and sends their people to. It runs
// no tasks itself.
func cloudCommand() int {
	front, controller, err := openCloud()
	if err != nil {
		return fail(err)
	}
	defer front.Store.Close()
	address := setting("GUSTO_CLOUD_ADDR", "127.0.0.1:8050")
	// There is no write timeout: an organization's task stream stays open for as long as
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
	if front.Seats != nil {
		go front.Seats.Run(ctx, time.Minute)
	}
	scaler, err := cellScaler(controller)
	if err != nil {
		return fail(err)
	}
	if scaler != nil {
		controller.OnWaiting = scaler.Wake
		front.Prewarm = scaler.Prewarm
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
	fmt.Printf("gusto cloud is listening on %s, with its accounts in %s\n", address, front.Store.Database())
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	front.Wait()
	<-controlled
	return 0
}

// openCloud reads gusto cloud's settings and opens what it keeps. Links
// that expired are swept when it starts and every hour after. A database that has no accounts yet
// takes in the files an earlier release kept under the root.
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
	limits.Paid.Jobs = number("GUSTO_CLOUD_PAID_JOBS_PER_SEAT", limits.Paid.Jobs)
	limits.Paid.Members = number("GUSTO_CLOUD_PAID_MEMBERS", limits.Paid.Members)
	limits.Paid.Computers = limits.Paid.Members
	stripe, err := stripeSettings()
	if err != nil {
		return nil, nil, err
	}
	store, err := cloud.Open(root, cloud.Settings{
		AllowFreeMail:  os.Getenv("GUSTO_CLOUD_FREE_MAIL") == "allow",
		RefusedDomains: lowered(values("GUSTO_CLOUD_REFUSED_DOMAINS")),
		PerDomain:      number("GUSTO_CLOUD_PER_DOMAIN", 3),
		Limits:         limits,
		Demo:           demoSettings(),
		DatabaseURL:    cloudDatabaseURL(),
	})
	if err != nil {
		return nil, nil, err
	}
	if err := store.ApplyQuotas(); err != nil {
		store.Close()
		return nil, nil, err
	}
	sessions, err := store.OpenSessions(public.Scheme == "https")
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	mail, err := cloud.MailFrom(os.Getenv("GUSTO_CLOUD_MAIL"), filepath.Join(root, "outbox"), os.Getenv("GUSTO_CLOUD_MAIL_TOKEN"))
	if err != nil {
		return nil, nil, err
	}
	domain := setting("GUSTO_CLOUD_DOMAIN", "localhost")
	var controller *cloud.Controller
	if os.Getenv("GUSTO_CLOUD_SERVERS") != "off" {
		if controller, err = organizationServers(store, public, limits); err != nil {
			return nil, nil, err
		}
		controller.OrgURL = func(slug string) string { return cloud.OrgAddress(public, domain, slug) }
	}
	var lease *cloud.Lease
	if path := os.Getenv("GUSTO_CLOUD_LEASE_FILE"); path != "" {
		lease = cloud.OpenLease(path, time.Duration(number("GUSTO_CLOUD_LEASE_STEP_MINUTES", 30))*time.Minute)
	}
	github, err := githubSignIn(context.Background(), os.Getenv("GUSTO_CONNECTOR_CLIENTS"), openParameters)
	if err != nil {
		return nil, nil, err
	}
	front, err := cloud.New(cloud.Config{
		Lease: lease, Store: store, Mail: mail, Sessions: sessions, URL: public, Controller: controller,
		Domain:     domain,
		TrustProxy: os.Getenv("GUSTO_CLOUD_TRUST_PROXY") == "1",
		Notify:     os.Getenv("GUSTO_CLOUD_DEMO_NOTIFY"),
		Leads:      &cloud.LeadHook{URL: os.Getenv("GUSTO_CLOUD_LEAD_HOOK_URL"), Secret: os.Getenv("GUSTO_CLOUD_LEAD_HOOK_SECRET"), Dir: filepath.Join(root, "leadhook")},
		Turnstile:  cloud.Turnstile{Site: os.Getenv("GUSTO_CLOUD_TURNSTILE_SITE"), Secret: os.Getenv("GUSTO_CLOUD_TURNSTILE_SECRET")},
		PerAddress: number("GUSTO_CLOUD_PER_ADDRESS", 0), PerMailbox: number("GUSTO_CLOUD_PER_MAILBOX", 0), InAll: number("GUSTO_CLOUD_IN_ALL", 0),
		Stripe: stripe, GitHub: github,
	})
	if err != nil {
		return nil, nil, err
	}
	if stripe != nil {
		front.Seats = cloud.NewSeatSync(store, stripe)
	}
	go sweepLinks(store)
	return front, controller, nil
}

// githubSignIn is the sign-in with GitHub, when the operator's clients include GitHub's: the same OAuth app people connect
// GitHub to their agents with signs them in to gusto cloud.
func githubSignIn(ctx context.Context, setting string, open func(context.Context) (parameterReader, error)) (*cloud.GitHub, error) {
	clients, err := connectorClients(ctx, setting, open)
	if err != nil {
		return nil, err
	}
	client, found := clients["github"]
	if !found || client.ID == "" || client.Secret == "" {
		return nil, nil
	}
	return &cloud.GitHub{ID: client.ID, Secret: client.Secret}, nil
}

// stripeSettings is how gusto cloud bills seats, and nil when it bills nothing. Setting some of
// the three and not all is refused, so a front door never takes payments it cannot hear about.
func stripeSettings() (*cloud.Stripe, error) {
	s := &cloud.Stripe{
		Key: os.Getenv("GUSTO_CLOUD_STRIPE_KEY"), Price: os.Getenv("GUSTO_CLOUD_STRIPE_PRICE"),
		WebhookSecret: os.Getenv("GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET"), SeatUSD: number("GUSTO_CLOUD_SEAT_USD", 25),
	}
	switch {
	case s.Enabled():
		return s, nil
	case s.Key == "" && s.Price == "" && s.WebhookSecret == "":
		return nil, nil
	}
	return nil, errors.New("billing needs GUSTO_CLOUD_STRIPE_KEY, GUSTO_CLOUD_STRIPE_PRICE and GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET together")
}

// organizationServers is what starts each organization's gusto serve on this machine.
// GUSTO_CLOUD_ORG_ENV names a file of KEY=VALUE lines that every organization's
// server is started with, which is where the operator's model credentials, models and
// sandbox settings go. Nothing else of gusto cloud's environment goes to them.
func organizationServers(store *cloud.Store, public *url.URL, limits cloud.Limits) (*cloud.Controller, error) {
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
		IndexDir: setting("GUSTO_CLOUD_INDEX_DIR", filepath.Join(store.Root(), "index")),
	}
	pool, err := cellPool(local)
	if err != nil {
		return nil, err
	}
	if pool != nil {
		controller.Cells = pool
		// A host apart runs the servers from the shared files, as this release, and not the program it was made with.
		shared, err := cloud.ShareRelease(exe, filepath.Join(store.Root(), "releases"))
		if err != nil {
			return nil, fmt.Errorf("this release could not be put where the hosts run it from: %w", err)
		}
		controller.Command = []string{shared, "serve"}
	}
	return controller, nil
}

// operatorEnv reads the settings every organization's server gets. A line is
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
// most at once (5), GUSTO_CLOUD_DEMO_PER_DAY for the most in a day (40), GUSTO_CLOUD_DEMO_PER_PERSON and GUSTO_CLOUD_DEMO_PER_COMPANY for the most one address (2) and one company (6) may start in twenty-four hours, GUSTO_CLOUD_DEMO_NOTIFY for an address that is mailed when a visitor starts a demo, GUSTO_CLOUD_DEMO_VIDEO for the video the landing page shows (an https address of a file or of a page to embed), GUSTO_CLOUD_DEMO_STAFF for the addresses and domains whose people alone may start a demo (anyone, when it is not set), and GUSTO_CLOUD_BOOK_URL for where a call is booked.
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
		Staff:      lowered(values("GUSTO_CLOUD_DEMO_STAFF")),
	}
}
