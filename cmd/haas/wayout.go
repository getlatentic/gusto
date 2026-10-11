package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/openshell"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/redact"
	"github.com/getlatentic/haas/internal/registry"
	"github.com/getlatentic/haas/internal/rules"
	"github.com/getlatentic/haas/internal/runner"
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
	// held is every credential this deployment holds now, which changes when an organisation's registry does.
	held atomic.Pointer[proxy.Credentials]
	// platform is what the operator configured, which a registry is added to.
	platform registry.Platform
	direct   string
	// sealed containers have no network, and reach the proxy through socket.
	// An unsealed one reaches it on address, over the runtime's own network.
	sealed  bool
	socket  string
	address string
	// gateways is the tool servers a job reaches through the proxy.
	gateways *gateways
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
	credentials, err := heldCredentials()
	if err != nil {
		return nil, err
	}
	if len(credentials) == 0 {
		return &WayOut{direct: direct, ledger: ledger}, nil
	}
	platform := registry.Platform{Credentials: credentials, Models: values("HAAS_MODELS"), Prices: policy.Prices}
	tokens := proxy.NewTokens()
	where := func(id string) (string, bool) {
		if strings.ContainsAny(id, `/\`) {
			return "", false
		}
		return filepath.Join(jobsRoot, id, "out", "proxy.jsonl"), true
	}
	way := &WayOut{
		proxy:    proxy.New(tokens, credentials, where, ledger, proxy.ParseAllowlist(os.Getenv("HAAS_ALLOWED_HOSTS"))),
		platform: platform,
		tokens:   tokens,
		ledger:   ledger,
		sealed:   os.Getenv("HAAS_JOB_NETWORK") != "open" && os.Getenv("HAAS_SANDBOX") != "openshell" && os.Getenv("HAAS_SANDBOX") != "here",
		socket:   setting("HAAS_PROXY_SOCKET", filepath.Join(jobsRoot, ".proxy.sock")),
		address:  setting("HAAS_PROXY_ADDR", proxyAddress()),
	}
	way.held.Store(&credentials)
	way.gateways, err = openGateways(jobsRoot)
	if err != nil {
		return nil, err
	}
	way.proxy.UseGateways(way.gateways.For)
	return way, nil
}

// credentials is every credential this deployment holds now.
func (w *WayOut) credentials() proxy.Credentials {
	if held := w.held.Load(); held != nil {
		return *held
	}
	return nil
}

// UseRedaction sets what is masked in what passes through the proxy and the gateway, from the next call on.
func (w *WayOut) UseRedaction(redactor *redact.Redactor) {
	if w.proxy == nil {
		return
	}
	w.proxy.UseRedaction(redactor)
	w.gateways.redactor.Store(redactor)
}

// SetCredentials changes what the proxy holds from the next call on, for a deployment with no registry. A call in flight keeps the
// credential it began with.
func (w *WayOut) SetCredentials(credentials proxy.Credentials) {
	if w.proxy == nil {
		return
	}
	w.held.Store(&credentials)
	w.proxy.SetCredentials(credentials, 0)
}

// Apply makes what an organisation's registry comes to what the proxy holds and the ledger prices at, from the
// next call on. A job already running keeps the credential it began with, which the proxy finds again by name.
func (w *WayOut) Apply(effective registry.Effective) {
	w.held.Store(&effective.Credentials)
	w.proxy.SetCredentials(effective.Credentials, effective.Version)
	w.ledger.SetPrices(effective.Prices)
}

// Listen starts the proxy where the containers will look for it: a socket for
// sealed ones, and a port for a runtime that cannot be handed a socket. What
// keeps the port safe is the per-job token and the host's firewall, not the
// interface it binds.
// ProxyCounts is what the proxy has counted, or nil when there is no proxy.
func (w *WayOut) ProxyCounts() func() proxy.Counts {
	if w.proxy == nil {
		return nil
	}
	return w.proxy.Counts
}

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
	return w.RunContext(context.Background(), dir, spec, mounts...)
}

// RunContext is Run that stops the container when its context ends.
func (w *WayOut) RunContext(ctx context.Context, dir job.Dir, spec job.Spec, mounts ...sandbox.Mount) (int, error) {
	if w.proxy == nil {
		return sandbox.RunContext(ctx, dir, w.direct, "", mounts...)
	}
	token, err := w.tokenFor(dir, spec)
	if err != nil {
		return 0, err
	}
	if w.sealed {
		return sandbox.SealedContext(ctx, dir, w.socket, token, mounts...)
	}
	_, port, err := net.SplitHostPort(w.address)
	if err != nil {
		port = "8041"
	}
	return sandbox.RunContext(ctx, dir, "http://localhost:"+port+"/model/v1", token, mounts...)
}

// tokenFor is the token a job's box is given, or none when it needs neither a model nor the gateway. A job that needs no model but
// has servers behind the gateway is given a token that stands in for no credential: it is how the proxy knows whose servers to call.
func (w *WayOut) tokenFor(dir job.Dir, spec job.Spec) (string, error) {
	switch {
	case needsModel(dir, spec):
		return w.admit(dir, spec)
	case spec.Setup != nil && len(spec.Setup.Gateway) > 0:
		return w.tokens.Mint(filepath.Base(dir.Path()), "")
	}
	return "", nil
}

// needsModel is whether a job's harness asks a model at all. One that needs none, such as the program the smoke check
// runs, is not refused for want of a credential, and is given no token.
func needsModel(dir job.Dir, spec job.Spec) bool {
	configured, err := harness.Open(dir, spec.Harness)
	return err != nil || configured.Adapter.Needs != harness.NeedsNothing
}

// admit checks that the job may reach its model, and gives it the token that
// stands in for the credential. It is refused before anything starts, by name,
// rather than as a model call that fails somewhere inside a run somebody has
// to go and read.
func (w *WayOut) admit(dir job.Dir, spec job.Spec) (string, error) {
	id := filepath.Base(dir.Path())
	named, _ := proxy.Named(spec.Model)
	credential, held := w.credentials().Get(named)
	if !held {
		return "", fmt.Errorf("no credential called %q here; this deployment holds %s",
			named, strings.Join(w.credentials().Names(), ", "))
	}
	// Every call a sealed box makes to a model leaves through this credential,
	// so its address is where the code goes. The runner inside checks the
	// address it was handed, which is the proxy on its own loopback, and would
	// read a restricted job as staying in the building when it does not.
	if err := rules.Allows(spec, credential.Address); err != nil {
		return "", runner.Refuse(dir, err)
	}
	w.ledger.Begin(id, spend.TermsOf(spec))
	return w.tokens.Mint(id, named)
}

// RunInSandbox runs a job's harness in an OpenShell sandbox. The proxy is
// reached on the name the sandbox gives this machine, so it listens on a port
// and not a socket, and the runner stays here and writes the record.
func (w *WayOut) RunInSandbox(dir job.Dir, spec job.Spec, cfg openshell.Config) (int, error) {
	return w.RunInSandboxContext(context.Background(), dir, spec, cfg)
}

// RunInSandboxContext is RunInSandbox that stops the run when its context ends.
func (w *WayOut) RunInSandboxContext(ctx context.Context, dir job.Dir, spec job.Spec, cfg openshell.Config) (int, error) {
	if w.proxy == nil {
		return 0, runner.Refuse(dir, errors.New("a sandbox on a gateway reaches its model through the proxy, and this deployment holds no credential for one"))
	}
	token, err := w.admit(dir, spec)
	if err != nil {
		return 0, err
	}
	port := w.proxyPort()
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return 0, err
	}
	box, err := openshell.New(cfg, filepath.Base(dir.Path()), configured.Adapter,
		"http://"+cfg.Host+":"+port+"/model/v1", "http://127.0.0.1:"+port+"/model/v1", token)
	if err != nil {
		return 0, err
	}
	return runner.OutsideContext(ctx, dir, box)
}

// proxyPort is the port the proxy listens on.
func (w *WayOut) proxyPort() string {
	if _, port, err := net.SplitHostPort(w.address); err == nil {
		return port
	}
	return "8041"
}

// Done ends a job's way out, so a container that outlives its server cannot
// keep spending.
func (w *WayOut) Done(dir job.Dir) {
	if w.proxy != nil {
		id := filepath.Base(dir.Path())
		w.tokens.Drop(id)
		w.gateways.Forget(id)
	}
}

// heldCredentials is every credential this deployment holds. HAAS_CREDENTIALS
// names a directory of <name>.json, the way the keyring and the adapters are
// read. A deployment configured before that directory existed named one
// upstream in the environment, and it is read as the one unnamed credential
// that serves every job.
func heldCredentials() (proxy.Credentials, error) {
	if dir := os.Getenv("HAAS_CREDENTIALS"); dir != "" {
		return proxy.OpenCredentials(dir)
	}
	address := os.Getenv("HAAS_MODEL_UPSTREAM")
	if address == "" {
		return nil, nil
	}
	return proxy.Credentials{"": {
		Address: address,
		Key:     os.Getenv("HAAS_MODEL_KEY"),
		Name:    setting("HAAS_MODEL_NAME", "the platform credential"),
	}}, nil
}

// InBuilding says whether any model this deployment reaches is inside its own
// network, which is the only place restricted work may go.
func (w *WayOut) InBuilding() bool {
	if w.proxy == nil {
		return rules.Local(w.direct)
	}
	held := w.credentials()
	for _, name := range held.Names() {
		if credential, found := held.Get(name); found && rules.Local(credential.Address) {
			return true
		}
	}
	return false
}

// proxyAddress is where the proxy listens unless it is told. A container reaches
// it over the runtime's network, so it listens on every interface; a sandbox on
// a gateway reaches the machine's loopback through the gateway's own proxy, so
// it need not be on the network at all.
func proxyAddress() string {
	if sandbox := os.Getenv("HAAS_SANDBOX"); sandbox == "openshell" || sandbox == "here" {
		return "127.0.0.1:8041"
	}
	return "0.0.0.0:8041"
}
