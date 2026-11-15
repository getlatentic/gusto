package main

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/settings"

	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/internal/rules"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/runtoken"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/redact"
	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/task"
)

// wayOutSettings is how a turn reaches its model and its tools: the credentials the proxy holds, the prices and caps, where the
// proxy listens, the tool gateway, the files read again as they change, the models offered, and the registry and connectors
// an organization's admins keep.
type wayOutSettings struct {
	modelURL, upstream, modelKey, modelName, credentials string
	spendPolicy, allowedHosts, proxySocket, proxyAddr    string
	maxOutput, silenceSeconds                            int
	toolPolicy, gatewayMode                              string
	gatewayPrivate                                       bool
	redaction, live                                      string
	reloadSeconds, limitPerson, limitTeam                int
	models                                               []string
	registry, registryPrivate, keychain                  string
	registryKeys                                         []string
	connectors, connectorClients, connectorApps          string
}

func (w *wayOutSettings) declare(set *settings.Set) {
	w.declareModels(set)
	w.declareProxy(set)
	w.declareTools(set)
	w.declareLive(set)
	w.declareRegistry(set)
}

func (w *wayOutSettings) declareModels(set *settings.Set) {
	set.String(&w.modelURL, "GUSTO_MODEL_URL", "http://localhost:11434/v1", "the model a task asks directly when this server holds no credential")
	set.String(&w.upstream, "GUSTO_MODEL_UPSTREAM", "", "the root of the one model provider this server holds a key for, with no version on it")
	set.Secret(&w.modelKey, "GUSTO_MODEL_KEY", "the key for GUSTO_MODEL_UPSTREAM, which the proxy holds")
	set.String(&w.modelName, "GUSTO_MODEL_NAME", "the platform credential", "what the credential for GUSTO_MODEL_UPSTREAM is called")
	set.String(&w.credentials, "GUSTO_CREDENTIALS", "", "the directory of credentials the proxy holds, one <name>.json each")
	set.String(&w.spendPolicy, "GUSTO_SPEND_POLICY", "", "the file of prices and team caps")
	set.List(&w.models, "GUSTO_MODELS", "the models offered, as credential/model, comma-separated")
}

func (w *wayOutSettings) declareProxy(set *settings.Set) {
	set.String(&w.allowedHosts, "GUSTO_ALLOWED_HOSTS", "", "every other host a task may reach through the proxy, comma-separated")
	set.String(&w.proxySocket, "GUSTO_PROXY_SOCKET", "", "the socket a box with no network reaches the proxy on (default .proxy.sock in GUSTO_TASKS)")
	set.String(&w.proxyAddr, "GUSTO_PROXY_ADDR", "", "where the proxy listens for a box on a network (default port 8041, on every interface for containers and the loopback otherwise)")
	set.Int(&w.maxOutput, "GUSTO_PROXY_MAX_OUTPUT_TOKENS", 0, 1, "the most output tokens one model call may ask for (default no limit)")
	set.Int(&w.silenceSeconds, "GUSTO_PROXY_SILENCE_SECONDS", 0, 1, "how long a model may send nothing before its call is ended, in seconds (default no limit)")
}

func (w *wayOutSettings) declareTools(set *settings.Set) {
	set.String(&w.toolPolicy, "GUSTO_TOOL_POLICY", "", "the file of which tool servers a task may call, and how")
	set.Choice(&w.gatewayMode, "GUSTO_GATEWAY_MODE", "discover", []string{"discover", "all"}, "how a task finds the tools behind the gateway")
	set.Bool(&w.gatewayPrivate, "GUSTO_GATEWAY_PRIVATE", "let tool servers and connected apps be on a private network")
}

func (w *wayOutSettings) declareLive(set *settings.Set) {
	set.String(&w.redaction, "GUSTO_REDACTION", "", "the file of what the proxy and the gateway mask in what passes through")
	set.String(&w.live, "GUSTO_LIVE", "", "the file of limits and models read again as it changes")
	set.Int(&w.reloadSeconds, "GUSTO_RELOAD_SECONDS", 20, 1, "how often the files read as they change are looked at, in seconds")
	set.Int(&w.limitPerson, "GUSTO_LIMIT_PERSON", 0, 0, "the most tasks one person may have running (default no limit)")
	set.Int(&w.limitTeam, "GUSTO_LIMIT_TEAM", 0, 0, "the most tasks may run for the whole team (default no limit)")
}

func (w *wayOutSettings) declareRegistry(set *settings.Set) {
	set.String(&w.registry, "GUSTO_REGISTRY", "", "the directory of the model providers and models an organization's admins keep")
	set.List(&w.registryKeys, "GUSTO_REGISTRY_KEYS", "the kinds of reference a provider's key may be besides one typed in: file, ssm, secretsmanager")
	set.Choice(&w.registryPrivate, "GUSTO_REGISTRY_PRIVATE", "", []string{"allow"}, "allow lets a provider be at a private address or plain http")
	set.Choice(&w.keychain, "GUSTO_KEYCHAIN", "on", []string{"on", "off"}, "off keeps model keys in files only you can read, not in the keychain")
	set.String(&w.connectors, "GUSTO_CONNECTORS", "", "the directory people's connected apps are kept in")
	set.String(&w.connectorClients, "GUSTO_CONNECTOR_CLIENTS", "", "the file of the clients registered with the apps that need one, or ssm:<name>")
	set.String(&w.connectorApps, "GUSTO_CONNECTOR_APPS", "", "the file of more apps than the catalog's")
}

// WayOut is how a container reaches a model. When a credential is configured it
// is the proxy, which holds the key, writes down what actually went, and lets a
// task reach nothing else unless the allowlist names it. When one is not, it is
// the model's own address, which is the local case where there is no credential
// to keep out of anywhere.
type WayOut struct {
	cfg    wayOutSettings
	proxy  *proxy.Proxy
	tokens *proxy.Tokens
	ledger *spend.Ledger
	// held is every credential this deployment holds now, which changes when an organization's registry does.
	held atomic.Pointer[proxy.Credentials]
	// platform is what the operator configured, which a registry is added to.
	platform registry.Platform
	direct   string
	// socket is where a box with no network reaches the proxy, and address where any other box does. An empty address is
	// the proxy's port on the interfaces that suit the boxes.
	socket  string
	address string
	// gateways is the tool servers a task reaches through the proxy.
	gateways *gateways
	// plainRuns says whether a run in a sandbox gets a plain token in place of a secret its forwarder trades for a run token
	// bound to a key its sandbox holds; see UseRunTokens. Unset, every run is bound.
	plainRuns atomic.Pointer[func() bool]
	// asker is the person a task acts for, by task, which a run's token names as its subject; see UseAskers.
	asker atomic.Pointer[func(id string) string]
	// describe says who and what a run is for, for its record; see UseRunRecords.
	describe atomic.Pointer[func(task.Dir, task.Spec) record.Run]
}

// UseRunRecords says how to describe a run for its signed record: who asked, who pays, and the room's agent, which the server's
// index and rooms know.
func (w *WayOut) UseRunRecords(describe func(task.Dir, task.Spec) record.Run) {
	w.describe.Store(&describe)
}

// RecordRun keeps, in the run's record, who and what it is for, the sandbox it runs in and how it was given its model token.
func (w *WayOut) RecordRun(dir task.Dir, spec task.Spec, sandbox string, bound bool) error {
	run := record.Run{Room: spec.Session, Agent: spec.Agent}
	if describe := w.describe.Load(); describe != nil {
		run = (*describe)(dir, spec)
	}
	run.Sandbox, run.ModelToken = sandbox, record.TokenPlain
	if bound {
		run.ModelToken = record.TokenBound
	}
	return record.WriteRun(dir, run)
}

// UseRunTokens says how to find whether the organization's admins chose plain tokens for runs in sandboxes, which is asked as
// each run starts.
func (w *WayOut) UseRunTokens(plain func() bool) { w.plainRuns.Store(&plain) }

func (w *WayOut) bindsRuns() bool {
	if plain := w.plainRuns.Load(); plain != nil {
		return !(*plain)()
	}
	return true
}

// UseAskers says how to find the person a task acts for, which the server's index knows.
func (w *WayOut) UseAskers(asker func(id string) string) { w.asker.Store(&asker) }

// UseMemberBudgets says what each person may spend in a month. A task is held to the budget of the person who asked for it, as
// it stands at each call.
func (w *WayOut) UseMemberBudgets(budget func(person string) (float64, bool)) {
	w.ledger.UsePersonBudgets(budget)
}

func (w *WayOut) askerOf(id string) string {
	if asker := w.asker.Load(); asker != nil {
		return (*asker)(id)
	}
	return ""
}

// openWayOut opens the way out cfg describes, with the ledger starting from what every task under tasksRoot has already spent. A
// deployment that holds no credential has no proxy, and its boxes ask the model directly, except on a person's own computer,
// which always has one, since the person adds their models to it as they go.
func openWayOut(tasksRoot string, cfg wayOutSettings, local bool) (*WayOut, error) {
	policy, err := spend.LoadPolicy(cfg.spendPolicy)
	if err != nil {
		return nil, err
	}
	// The ledger exists without a proxy too, so the spend page reports what tasks spent through one before, rather than
	// nothing.
	ledger, err := spend.Rebuild(tasksRoot, policy)
	if err != nil {
		return nil, err
	}
	credentials, err := heldCredentials(cfg)
	if err != nil {
		return nil, err
	}
	if len(credentials) == 0 && !local {
		return &WayOut{cfg: cfg, direct: cfg.modelURL, ledger: ledger}, nil
	}
	tokens := proxy.NewTokens()
	way := &WayOut{
		cfg:      cfg,
		direct:   cfg.modelURL,
		proxy:    proxy.New(tokens, credentials, recordsUnder(tasksRoot), ledger, proxy.ParseAllowlist(cfg.allowedHosts)),
		platform: registry.Platform{Credentials: credentials, Models: cfg.models, Prices: policy.Prices},
		tokens:   tokens,
		ledger:   ledger,
		socket:   firstOf(cfg.proxySocket, filepath.Join(tasksRoot, ".proxy.sock")),
		address:  cfg.proxyAddr,
	}
	way.held.Store(&credentials)
	return way, way.equipProxy(tasksRoot)
}

// recordsUnder is where the proxy writes down what a task sent: proxy.jsonl in the task's out/, under tasksRoot.
func recordsUnder(tasksRoot string) func(id string) (string, bool) {
	return func(id string) (string, bool) {
		if strings.ContainsAny(id, `/\`) {
			return "", false
		}
		return filepath.Join(tasksRoot, id, "out", "proxy.jsonl"), true
	}
}

// equipProxy gives the proxy how far one call may run, the run tokens a sandbox's forwarder trades for, and the tool gateway.
func (w *WayOut) equipProxy(tasksRoot string) error {
	w.proxy.UseLimits(proxy.Limits{MaxOutput: w.cfg.maxOutput, Silence: time.Duration(w.cfg.silenceSeconds) * time.Second})
	issuer, err := runtoken.NewIssuer(runtoken.TokenLife(), func(id string) runtoken.Run { return w.runOf(tasksRoot, id) })
	if err != nil {
		return err
	}
	w.proxy.UseRunTokens(issuer)
	if w.gateways, err = openGateways(tasksRoot, w.cfg); err != nil {
		return err
	}
	w.proxy.UseGateways(w.gateways.For)
	return nil
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

// Apply makes what an organization's registry comes to what the proxy holds and the ledger prices at, from the
// next call on. A task already running keeps the credential it began with, which the proxy finds again by name.
func (w *WayOut) Apply(effective registry.Effective) {
	w.held.Store(&effective.Credentials)
	w.proxy.SetCredentials(effective.Credentials, effective.Version)
	w.ledger.SetPrices(effective.Prices)
}

// ProxyCounts is what the proxy has counted, or nil when there is no proxy.
func (w *WayOut) ProxyCounts() func() proxy.Counts {
	if w.proxy == nil {
		return nil
	}
	return w.proxy.Counts
}

// Listen starts the proxy where the boxes will look for it: a socket for boxes with no network, and a port for the rest.
// What keeps the port safe is the per-task token and the host's firewall, not the interface it binds.
func (w *WayOut) Listen(reach backend.Reach) (func(), error) {
	switch {
	case w.proxy == nil:
		return func() {}, nil
	case reach == backend.Socket:
		fmt.Printf("the only way out of a sandbox is %s, and the credential stays here\n", w.socket)
		return w.proxy.ListenSocket(w.socket)
	}
	address := listenAddress(w.address, reach)
	fmt.Printf("the way out of a sandbox is %s, and the credential stays here\n", address)
	return w.proxy.ListenTCP(address)
}

// listenAddress is where the proxy listens unless it is told. A container reaches it over the runtime's network, so it
// listens on every interface; a box on this computer, or a sandbox on a gateway, reaches the machine's loopback, so it need
// not be on the network at all.
func listenAddress(configured string, reach backend.Reach) string {
	switch {
	case configured != "":
		return configured
	case reach == backend.Network:
		return "0.0.0.0:8041"
	}
	return "127.0.0.1:8041"
}

// exit is how a turn's box reaches its model through this way out.
func (w *WayOut) exit() backend.Exit {
	exit := backend.Exit{Direct: w.direct, Socket: w.socket, Port: w.proxyPort()}
	if w.proxy != nil {
		exit.Door = w
	}
	return exit
}

// Token is the token a task's box is given, or none when it needs neither a model nor the gateway. A task that needs no model but
// has servers behind the gateway is given a token that stands in for no credential: it is how the proxy knows whose servers to call.
func (w *WayOut) Token(dir task.Dir, spec task.Spec) (string, error) {
	switch {
	case needsModel(dir, spec):
		return w.admit(dir, spec)
	case spec.Setup != nil && len(spec.Setup.Gateway) > 0:
		return w.tokens.Mint(filepath.Base(dir.Path()), "")
	}
	return "", nil
}

// needsModel is whether a task's agent asks a model at all. One that needs none, such as the program the smoke check
// runs, is not refused for want of a credential, and is given no token.
func needsModel(dir task.Dir, spec task.Spec) bool {
	configured, err := harness.Open(dir, spec.Harness)
	return err != nil || configured.Adapter.Needs != harness.NeedsNothing
}

// admit checks that the task may reach its model, and gives it the token that
// stands in for the credential. It is refused before anything starts, by name,
// rather than as a model call that fails somewhere inside a run somebody has
// to go and read.
func (w *WayOut) admit(dir task.Dir, spec task.Spec) (string, error) {
	id, named, err := w.mayReach(dir, spec)
	if err != nil {
		return "", err
	}
	return w.tokens.Mint(id, named)
}

// admitBound is admit for a task in a sandbox, which holds a key of its own: it is given a secret that its forwarder trades,
// once, for a run token bound to that key (internal/runtoken).
func (w *WayOut) admitBound(dir task.Dir, spec task.Spec) (string, error) {
	id, named, err := w.mayReach(dir, spec)
	if err != nil {
		return "", err
	}
	return w.tokens.MintBound(id, named)
}

// SandboxToken is admit for a task in a sandbox: with a secret for its forwarder when runs are bound, and a plain token
// otherwise. It says which.
func (w *WayOut) SandboxToken(dir task.Dir, spec task.Spec) (token string, bound bool, err error) {
	if w.bindsRuns() {
		token, err = w.admitBound(dir, spec)
		return token, true, err
	}
	token, err = w.admit(dir, spec)
	return token, false, err
}

// mayReach checks that a task may reach its model, and says the task and the credential it spends.
func (w *WayOut) mayReach(dir task.Dir, spec task.Spec) (string, string, error) {
	id := filepath.Base(dir.Path())
	named, _ := proxy.Named(spec.Model)
	credential, held := w.credentials().Get(named)
	if !held {
		return "", "", fmt.Errorf("no credential called %q here; this deployment holds %s",
			named, strings.Join(w.credentials().Names(), ", "))
	}
	// Every call a sealed box makes to a model leaves through this credential,
	// so its address is where the code goes. The runner inside checks the
	// address it was handed, which is the proxy on its own loopback, and would
	// read a restricted task as staying in the building when it does not.
	for _, address := range credential.Addresses() {
		if err := rules.Allows(spec, address); err != nil {
			return "", "", runner.Refuse(dir, err)
		}
	}
	w.ledger.Begin(id, spend.TermsOf(spec))
	return id, named, nil
}

// runOf is what a run's token says of it, from the task's own record: the agent and the room of a room's turn, and the person
// who asked, whom the run acts for. A task the server's index does not hold names nobody.
func (w *WayOut) runOf(tasksRoot, id string) runtoken.Run {
	spec, err := task.Read(task.Dir(filepath.Join(tasksRoot, id)))
	if err != nil {
		return runtoken.Run{Person: w.askerOf(id)}
	}
	agent := spec.Agent
	if agent == "" {
		agent = spec.Harness
	}
	return runtoken.Run{Agent: agent, Room: spec.Session, Person: w.askerOf(id)}
}

// proxyPort is the port the proxy listens on.
func (w *WayOut) proxyPort() string {
	if _, port, err := net.SplitHostPort(w.address); err == nil {
		return port
	}
	return "8041"
}

// Done ends a task's way out, so a container that outlives its server cannot
// keep spending.
func (w *WayOut) Done(dir task.Dir) {
	if w.proxy != nil {
		id := filepath.Base(dir.Path())
		w.tokens.Drop(id)
		w.gateways.Forget(id)
	}
}

// heldCredentials is every credential this deployment holds. GUSTO_CREDENTIALS
// names a directory of <name>.json, the way the keyring and the adapters are
// read. A deployment configured before that directory existed named one
// upstream in the environment, and it is read as the one unnamed credential
// that serves every task.
func heldCredentials(cfg wayOutSettings) (proxy.Credentials, error) {
	if cfg.credentials != "" {
		return proxy.OpenCredentials(cfg.credentials)
	}
	if cfg.upstream == "" {
		return nil, nil
	}
	return proxy.Credentials{"": {Address: cfg.upstream, Key: cfg.modelKey, Name: cfg.modelName}}, nil
}

// InBuilding says whether any model this deployment reaches is inside its own
// network, which is the only place restricted work may go.
func (w *WayOut) InBuilding() bool {
	if w.proxy == nil {
		return rules.Local(w.direct)
	}
	held := w.credentials()
	for _, name := range held.Names() {
		if credential, found := held.Get(name); found && allLocal(credential.Addresses()) {
			return true
		}
	}
	return false
}

func allLocal(addresses []string) bool {
	for _, address := range addresses {
		if !rules.Local(address) {
			return false
		}
	}
	return true
}
