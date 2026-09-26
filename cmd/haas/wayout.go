package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/sandbox"
	"github.com/getlatentic/haas/internal/spend"
)

// WayOut is how a container reaches a model. When a credential is configured it
// is the proxy, which holds the key, writes down what actually went, and lets a
// job reach nothing else unless the allowlist names it. When one is not, it is
// the model's own address, which is the local case where there is no credential
// to keep out of anywhere.
type WayOut struct {
	proxy  *proxy.Proxy
	tokens *proxy.Tokens
	ledger *spend.Ledger
	direct string
	// sealed containers have no network, and reach the proxy through socket.
	// An unsealed one reaches it on address, over the runtime's own network.
	sealed  bool
	socket  string
	address string
}

// openWayOut reads what is configured. HAAS_MODEL_UPSTREAM is the model's root,
// with no version on it, because the container supplies the rest of the path.
// HAAS_SPEND_POLICY names the prices and team caps, and the ledger starts from
// what every job under jobsRoot has already spent. HAAS_ALLOWED_HOSTS is every
// other host a job may reach.
func openWayOut(jobsRoot string) (*WayOut, error) {
	direct := setting("HAAS_MODEL_URL", "http://localhost:11434/v1")
	policy, err := spend.LoadPolicy(os.Getenv("HAAS_SPEND_POLICY"))
	if err != nil {
		return nil, err
	}
	// The ledger exists without a proxy too, so the spend page reports what
	// jobs spent through one before, rather than nothing.
	ledger, err := spend.Rebuild(jobsRoot, policy)
	if err != nil {
		return nil, err
	}
	upstream := os.Getenv("HAAS_MODEL_UPSTREAM")
	if upstream == "" {
		return &WayOut{direct: direct, ledger: ledger}, nil
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
		}, where, ledger, proxy.ParseAllowlist(os.Getenv("HAAS_ALLOWED_HOSTS"))),
		tokens:  tokens,
		ledger:  ledger,
		sealed:  os.Getenv("HAAS_JOB_NETWORK") != "open",
		socket:  setting("HAAS_PROXY_SOCKET", filepath.Join(jobsRoot, ".proxy.sock")),
		address: setting("HAAS_PROXY_ADDR", "0.0.0.0:8041"),
	}, nil
}

// Listen starts the proxy where the containers will look for it: a socket for
// sealed ones, and a port for a runtime that cannot be handed a socket. What
// keeps the port safe is the per-job token and the host's firewall, not the
// interface it binds.
func (w *WayOut) Listen() (func(), error) {
	switch {
	case w.proxy == nil:
		return func() {}, nil
	case w.sealed:
		fmt.Printf("the only way out of a sandbox is %s, and the credential stays here\n", w.socket)
		return w.proxy.ListenSocket(w.socket)
	}
	fmt.Printf("the way out of a sandbox is %s, and the credential stays here\n", w.address)
	return w.proxy.ListenTCP(w.address)
}

// Run starts a job's container on its way out. With a proxy the container is
// sealed and the proxy is all it reaches; without one it asks the model
// directly. The token is where a harness would have put a credential, so its
// configuration stays the same from one job to the next, and what this job
// spends is attributed to it and counted against its caps.
func (w *WayOut) Run(dir job.Dir, spec job.Spec, mounts ...sandbox.Mount) (int, error) {
	if w.proxy == nil {
		return sandbox.Run(dir, w.direct, "", mounts...)
	}
	id := filepath.Base(dir.Path())
	w.ledger.Begin(id, spend.TermsOf(spec))
	token, err := w.tokens.Mint(id)
	if err != nil {
		return 0, err
	}
	if w.sealed {
		return sandbox.Sealed(dir, w.socket, token, mounts...)
	}
	_, port, err := net.SplitHostPort(w.address)
	if err != nil {
		port = "8041"
	}
	return sandbox.Run(dir, "http://localhost:"+port+"/model/v1", token, mounts...)
}

// Done ends a job's way out, so a container that outlives its server cannot
// keep spending.
func (w *WayOut) Done(dir job.Dir) {
	if w.proxy != nil {
		w.tokens.Drop(filepath.Base(dir.Path()))
	}
}
