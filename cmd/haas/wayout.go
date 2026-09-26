package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/proxy"
)

// WayOut is how a container reaches a model. When a credential is configured it
// is the proxy, which holds the key and writes down what actually went; when one
// is not, it is the model's own address, which is the local case where there is
// no credential to keep out of anywhere.
type WayOut struct {
	proxy   *proxy.Proxy
	tokens  *proxy.Tokens
	address string
	direct  string
}

// openWayOut reads what is configured. HAAS_MODEL_UPSTREAM is the model's root,
// with no version on it, because the container supplies the rest of the path.
func openWayOut(jobsRoot string) (*WayOut, error) {
	direct := setting("HAAS_MODEL_URL", "http://localhost:11434/v1")
	upstream := os.Getenv("HAAS_MODEL_UPSTREAM")
	if upstream == "" {
		return &WayOut{direct: direct}, nil
	}
	tokens := proxy.NewTokens()
	where := func(id string) (string, bool) {
		if strings.ContainsAny(id, `/\`) {
			return "", false
		}
		return filepath.Join(jobsRoot, id, "out", "proxy.jsonl"), true
	}
	return &WayOut{
		proxy: proxy.New(tokens, proxy.Upstream{
			Address: upstream,
			Key:     os.Getenv("HAAS_MODEL_KEY"),
			Name:    setting("HAAS_MODEL_NAME", "the platform credential"),
		}, where),
		tokens:  tokens,
		address: setting("HAAS_PROXY_ADDR", "0.0.0.0:8041"),
	}, nil
}

// Listen starts the proxy's own listener.
//
// It cannot share the server's, because that binds loopback and no container can
// reach a loopback address on its host. What keeps this safe is the per-job
// token and the host's firewall, not the interface it binds.
func (w *WayOut) Listen() (func(), error) {
	if w.proxy == nil {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", w.address)
	if err != nil {
		return nil, fmt.Errorf("the proxy could not listen on %s: %w", w.address, err)
	}
	server := &http.Server{Handler: w.proxy.Handler()}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "haas: the proxy stopped: %v\n", err)
		}
	}()
	fmt.Printf("the way out of a sandbox is %s, and the credential stays here\n", w.address)
	return func() { _ = server.Close() }, nil
}

// For is the address this job's container should ask at. A job gets its own
// token, so what it spends is attributed to it and the address is worthless
// once the job ends.
func (w *WayOut) For(dir job.Dir) string {
	if w.proxy == nil {
		return w.direct
	}
	id := filepath.Base(dir.Path())
	token, err := w.tokens.Mint(id)
	if err != nil {
		return w.direct
	}
	_, port, err := net.SplitHostPort(w.address)
	if err != nil {
		port = "8041"
	}
	return "http://localhost:" + port + "/model/" + token + "/v1"
}

// Done ends a job's way out, so a container that outlives its server cannot
// keep spending.
func (w *WayOut) Done(dir job.Dir) {
	if w.proxy != nil {
		w.tokens.Drop(filepath.Base(dir.Path()))
	}
}
