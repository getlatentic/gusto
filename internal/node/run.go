package node

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// Run serves the agent on the address until the context ends, and says every few seconds that the node is
// there, so the gusto cloud places on it and reaches it. When it ends it stops the servers it started, all at once,
// which a node that is going has no use for, and takes its registration away. Stopping them together keeps the whole well
// inside the time the service is given to stop, however many there are.
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
			var stopping sync.WaitGroup
			for _, slug := range a.Servers.Running() {
				stopping.Go(func() { a.Servers.Stop(slug, orDefault(a.Grace, 30*time.Second)) })
			}
			stopping.Wait()
			return Unregister(dir, a.Name)
		}
	}
}
