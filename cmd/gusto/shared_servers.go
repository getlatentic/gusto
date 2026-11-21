package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/settings"
)

// sharedServers runs organizations' servers inside this process, as gusto cloud asks on the socket, until it is told to stop.
func sharedServers(args []string) int {
	if len(args) != 2 || args[0] != "--socket" || args[1] == "" {
		return usageError("shared-servers", "needs --socket <path>")
	}
	socket := args[1]
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	listener, err := listenOwnerOnly(socket)
	if err != nil {
		return fail(err)
	}
	host := &cloud.Host{Serve: serveHosted}
	server := &http.Server{Handler: host.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		host.StopAll(shutdownWithin)
		_ = server.Close()
	}()
	fmt.Println("gusto shared-servers is listening on", socket)
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	return 0
}

// listenOwnerOnly listens on a socket only this user may open: whoever opens it starts servers with the environment they send.
func listenOwnerOnly(socket string) (net.Listener, error) {
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

// serveHosted is one organization's server, read from env and its settings file as a process of its own reads its environment.
// A server for one person on their own computer changes the process's PATH, so it is never one of many.
func serveHosted(ctx context.Context, env []string) error {
	lookup, err := environmentOf(env)
	if err != nil {
		return err
	}
	if lookup("GUSTO_LOCAL") != "" {
		return errors.New("a server for one person on their own computer runs in a process of its own")
	}
	if code := runServer(ctx, lookup); code != 0 {
		return fmt.Errorf("the server stopped with exit code %d", code)
	}
	return nil
}

// environmentOf is the lookup of env, with what the settings file it names holds under it.
func environmentOf(env []string) (func(string) string, error) {
	values := map[string]string{}
	for _, each := range env {
		if name, value, found := strings.Cut(each, "="); found {
			values[name] = value
		}
	}
	lookup := func(name string) string { return values[name] }
	err := settings.ApplyFile(lookup, func(name, value string) error {
		values[name] = value
		return nil
	})
	return lookup, err
}
