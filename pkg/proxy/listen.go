package proxy

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// ListenSocket serves the proxy on a unix socket, which is what a sealed
// container is given instead of a network. The socket is left to its owner
// alone: rootful docker connects as root, and rootless podman as the user that
// runs it, which is the owner.
func (p *Proxy) ListenSocket(path string) (func(), error) {
	if err := clearSocket(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("the proxy could not listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	return p.serve(listener), nil
}

// ListenTCP serves the proxy on an address a container on the runtime's own
// network can reach, for a runtime that cannot be handed a socket. It cannot
// share the server's listener, because that binds loopback and no container can
// reach a loopback address on its host.
func (p *Proxy) ListenTCP(address string) (func(), error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("the proxy could not listen on %s: %w", address, err)
	}
	return p.serve(listener), nil
}

func (p *Proxy) serve(listener net.Listener) func() {
	// An agent may hold a connection open, but not send its headers slowly to tie one up. A model's answer streams for as
	// long as it runs, so a write has no limit.
	server := &http.Server{Handler: p.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("the proxy stopped", "error", err)
		}
	}()
	return func() { _ = server.Close() }
}

// clearSocket removes a socket a server that died left behind, and refuses to
// remove anything that is not a socket.
func clearSocket(path string) error {
	found, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if found.Mode()&fs.ModeSocket == 0 {
		return fmt.Errorf("%s is in the way of the proxy's socket and is not one", path)
	}
	return os.Remove(path)
}
