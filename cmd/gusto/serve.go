package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/accesslog"
	"github.com/getlatentic/gusto/internal/agentinstall"
	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/connector"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/internal/settings"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/internal/web"
)

// shutdownWithin is how long the server waits for live connections and requests to end once it is told to stop.
const shutdownWithin = 30 * time.Second

// server is what `gusto serve` is made of, in the order it is built: the records, the way out, the boxes turns run in and
// their queue, then the pages and what they use.
type server struct {
	cfg serveSettings
	// set is the settings as they were read, which the record of what changed is written from.
	set *settings.Set
	// signs stands behind the records of the tasks run here, and of what changes on the platform.
	signs signing.Identity

	index     *jobs.Index
	limits    *quota.File
	instances *instance.Store
	way       *WayOut
	shared    *apps.Store
	boxes     backend.Backend
	rooms     *roomRuns
	queue     *jobs.Queue
	pages     *web.Server
	installer *agentinstall.Installer
	connect   *connector.Connectors

	// stops are what each step started, ended in the reverse order they were started in.
	stops []func()
}

func serve(args []string) int {
	if len(args) != 0 {
		return usageError("serve", "takes no arguments")
	}
	return serveFrom(os.Getenv)
}

// serveFrom is the server with the settings lookup gives, until it stops.
func serveFrom(lookup func(string) string) int {
	s, code := newServer(lookup)
	if s == nil {
		return code
	}
	defer s.stop()
	if err := s.build(); err != nil {
		return fail(err)
	}
	if err := s.listen(s.cfg.core.addr); err != nil {
		return fail(err)
	}
	return 0
}

// newServer is a server that may start, or nil and the exit code of the reason it may not. A setting that does not read is a
// server started wrongly, which is said as a missing password is.
func newServer(lookup func(string) string) (*server, int) {
	cfg, set, err := loadServe(lookup)
	if err != nil {
		return nil, refused(err)
	}
	if cfg.core.password == "" {
		// A URL that spends money and runs code is not left open because a variable was forgotten.
		fmt.Fprintln(os.Stderr, "gusto: set GUSTO_PASSWORD; the server will not start without a sign-in")
		return nil, 2
	}
	accesslog.TrustForwarded(cfg.core.behindProxy)
	if cfg.core.local && !onLoopback(cfg.core.addr) {
		fmt.Fprintln(os.Stderr, "gusto: a server for one person on their own computer listens on the loopback only, and GUSTO_ADDR names another address")
		return nil, 2
	}
	if err := ready(cfg.stores); err != nil {
		return nil, fail(err)
	}
	signs, err := cfg.sign.identity()
	if err != nil {
		return nil, fail(err)
	}
	return &server{cfg: cfg, set: set, signs: signs}, 0
}

// build makes the server, step by step. A step that fails stops the build, and what the steps before it started is stopped.
func (s *server) build() error {
	for _, step := range []func() error{
		s.openRecords, s.openWayOut, s.startTurns,
		s.openPages, s.offerThisComputer, s.shareApps, s.applyOrgSettings, s.offerModels,
		s.offerSetups, s.offerHistory, s.startBackground, s.openDoors,
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// started keeps how to end what a step started.
func (s *server) started(stop func()) { s.stops = append(s.stops, stop) }

// stop ends what the steps started, the last first.
func (s *server) stop() {
	for i := len(s.stops) - 1; i >= 0; i-- {
		s.stops[i]()
	}
}

// listen serves the pages at address until the process is told to stop, and then closes the live connections and waits for
// requests in flight.
func (s *server) listen(address string) error {
	server := &http.Server{
		Addr:              address,
		Handler:           accesslog.Keep(accesslog.Stderr())(s.pages.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		// A task's stream stays open for as long as the task runs, so there is no write timeout to set here; nginx holds the
		// same view.
	}
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, syscall.SIGINT, syscall.SIGTERM)
	defer func() { signal.Stop(stopping); close(stopping) }()
	go func() {
		if _, told := <-stopping; !told {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), shutdownWithin)
		defer cancel()
		// A hijacked connection is not the HTTP server's to wait for, so the sockets are closed, and waited for, first.
		if err := s.pages.CloseLive(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "gusto: the live connections did not all close in time:", err)
		}
		_ = server.Shutdown(ctx)
	}()
	fmt.Printf("gusto is listening on %s, with %d task(s) at a time\n", address, s.cfg.boxes.concurrency)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// appHost is a backend that runs the web apps agents built where the agents are.
type appHost interface {
	Apps() apps.Runner
}
