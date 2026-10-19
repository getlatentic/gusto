package cell

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// Run serves the agent on the address until the context ends, and says every few seconds that the host is
// there, so the gusto cloud places on it and reaches it. When it ends it stops the servers it started,
// which a host that is going has no use for, and takes its registration away.
func (a *Agent) Run(ctx context.Context, listen, dir string, every time.Duration) error {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	registration := Registration{Name: a.Name, Agent: listener.Addr().String(), Private: a.Private}
	beat := func() {
		registration.Beat = a.clock().Unix()
		if err := Register(dir, registration); err != nil {
			slog.Error("the host could not say it is there", "error", err)
		}
	}
	beat()
	server := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			beat()
		case err := <-served:
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			server.Shutdown(shutdown)
			cancel()
			for _, slug := range a.Servers.Running() {
				a.Servers.Stop(slug, 30*time.Second)
			}
			if err := os.Remove(file(dir, a.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
	}
}
