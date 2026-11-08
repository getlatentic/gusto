package main

import (
	"github.com/getlatentic/gusto/internal/tasks"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/backend"
	"github.com/getlatentic/gusto/internal/settings"
	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/task"
)

// A sealed box reaches its model only through the proxy, on its own loopback,
// so the runner inside cannot tell where the code goes. The server can: the
// credential names the provider. Restricted code bound for a provider outside
// the building is refused before a box starts, and says so where the task is
// read.
func TestRestrictedCodeIsRefusedACredentialOutsideTheBuilding(t *testing.T) {
	credentials := t.TempDir()
	os.WriteFile(filepath.Join(credentials, "deepseek.json"),
		[]byte(`{"address": "https://api.deepseek.com", "key": "sk-test"}`), 0o600)
	t.Setenv("GUSTO_CREDENTIALS", credentials)
	way, err := openWayOut(t.TempDir(), fromEnv(t).way, fromEnv(t).core.local)
	if err != nil {
		t.Fatal(err)
	}
	dir := task.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := task.Spec{Brief: "b", Harness: "opencode", Model: "deepseek/deepseek-flash", Sensitivity: task.Restricted}
	if _, err := way.Token(dir, spec); err == nil || !strings.Contains(err.Error(), "api.deepseek.com") {
		t.Fatalf("restricted code was not refused the provider: %v", err)
	}
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	if !strings.Contains(string(said), "restricted") {
		t.Errorf("the refusal is not where the job is read: %q", said)
	}
}

// A credential whose first address is inside the building and whose second is not still sends restricted code outside it,
// since which address a call goes to is the agent's choice of API.
func TestRestrictedCodeIsRefusedACredentialWithAnyAddressOutsideTheBuilding(t *testing.T) {
	credentials := t.TempDir()
	os.WriteFile(filepath.Join(credentials, "inside.json"), []byte(`{"address": "http://127.0.0.1:9", "key": "k",
		"endpoints": [{"address": "https://api.deepseek.com/anthropic", "auth": "anthropic"}]}`), 0o600)
	t.Setenv("GUSTO_CREDENTIALS", credentials)
	way, err := openWayOut(t.TempDir(), fromEnv(t).way, fromEnv(t).core.local)
	if err != nil {
		t.Fatal(err)
	}
	if way.InBuilding() {
		t.Error("a credential with an address outside was said to be inside the building")
	}
	dir := task.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := task.Spec{Brief: "b", Harness: "claude", Model: "inside/model", Sensitivity: task.Restricted}
	if _, err := way.Token(dir, spec); err == nil || !strings.Contains(err.Error(), "api.deepseek.com") {
		t.Fatalf("restricted code was not refused the second address: %v", err)
	}
}

// A harness that needs no model has no credential to name, and a deployment that holds credentials still runs it, as
// the install check's first task does. A harness that does need one is still asked to name it.
func TestAHarnessThatNeedsNoModelIsNotRefusedForWantOfACredential(t *testing.T) {
	harnesses := t.TempDir()
	os.WriteFile(filepath.Join(harnesses, "plain.json"), []byte(`{"name":"plain","program":"true","needs":"nothing","stream":"plain"}`), 0o600)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := task.Dir(t.TempDir())
	if needsModel(dir, task.Spec{Harness: "plain"}) {
		t.Error("a harness that needs nothing was said to need a model")
	}
	if !needsModel(dir, task.Spec{Harness: "opencode"}) || !needsModel(dir, task.Spec{Harness: "nowhere"}) {
		t.Error("a harness that needs a model, or is not found, was said to need none")
	}
}

// The proxy listens where the boxes reach it: on every interface for a container on the runtime's network, and on the loopback
// for a turn in this process or a sandbox on a gateway, unless the operator says where.
func TestTheProxyListensWhereTheBoxesReachIt(t *testing.T) {
	for reach, want := range map[backend.Reach]string{backend.Network: "0.0.0.0:8041", backend.Loopback: "127.0.0.1:8041"} {
		if got := listenAddress("", reach); got != want {
			t.Errorf("reach %d listens on %s, want %s", reach, got, want)
		}
	}
	if got := listenAddress("10.0.0.4:9000", backend.Network); got != "10.0.0.4:9000" {
		t.Errorf("the operator's address was replaced by %s", got)
	}
}

// A turn's way out is the proxy and its port when a credential is held, and the model's own address when none is.
func TestATurnsWayOutIsTheProxyWhenACredentialIsHeld(t *testing.T) {
	credentials := t.TempDir()
	os.WriteFile(filepath.Join(credentials, "deepseek.json"), []byte(`{"address": "https://api.deepseek.com", "key": "sk-the-real-key"}`), 0o600)
	t.Setenv("GUSTO_CREDENTIALS", credentials)
	t.Setenv("GUSTO_PROXY_ADDR", "127.0.0.1:9041")
	way, err := openWayOut(t.TempDir(), fromEnv(t).way, fromEnv(t).core.local)
	if err != nil {
		t.Fatal(err)
	}
	if exit := way.exit(); exit.Door == nil || exit.Port != "9041" {
		t.Errorf("a deployment that holds a credential gave %+v", exit)
	}
	t.Setenv("GUSTO_CREDENTIALS", "")
	t.Setenv("GUSTO_MODEL_UPSTREAM", "")
	direct, err := openWayOut(t.TempDir(), fromEnv(t).way, fromEnv(t).core.local)
	if err != nil {
		t.Fatal(err)
	}
	if exit := direct.exit(); exit.Door != nil || exit.Direct == "" {
		t.Errorf("a deployment that holds no credential gave %+v", exit)
	}
}

func TestTheCallLimitsAreReadFromTheirSettingsAndAWrongOneIsSaid(t *testing.T) {
	t.Parallel()
	read := func(values map[string]string) (wayOutSettings, error) {
		var cfg wayOutSettings
		set := settings.New()
		cfg.declare(set)
		return cfg, set.Load(func(name string) string { return values[name] })
	}
	cfg, err := read(map[string]string{"GUSTO_PROXY_MAX_OUTPUT_TOKENS": "16384", "GUSTO_PROXY_SILENCE_SECONDS": "60"})
	if err != nil || cfg.maxOutput != 16384 || cfg.silenceSeconds != 60 {
		t.Fatalf("%+v %v", cfg, err)
	}
	for _, wrong := range []string{"a minute", "0"} {
		if _, err := read(map[string]string{"GUSTO_PROXY_SILENCE_SECONDS": wrong}); err == nil || !strings.Contains(err.Error(), "GUSTO_PROXY_SILENCE_SECONDS") {
			t.Errorf("%q was taken: %v", wrong, err)
		}
	}
}

func TestATaskIsHeldToTheBudgetOfThePersonWhoAskedForIt(t *testing.T) {
	way := &WayOut{ledger: spend.NewLedger(spend.Policy{})}
	way.UseMemberBudgets(func(person string) (float64, bool) { return 20, person == "ada@acme.com" })
	twenty := 20.0
	for _, person := range []string{"Ada@Acme.com", "bob@acme.com"} {
		way.ledger.Begin(person, spend.TermsOf(task.Spec{Person: person}))
		way.ledger.Count(person, spend.Call{Path: "/v1/chat/completions", Model: "m", Status: 200, AtUnix: time.Now().Unix(), USD: &twenty})
	}
	if refusal := way.ledger.Allow("Ada@Acme.com", "", "", false); refusal == nil || refusal.Cap != "person" {
		t.Errorf("ada's task went past her budget: %+v", refusal)
	}
	if refusal := way.ledger.Allow("bob@acme.com", "", "", false); refusal != nil {
		t.Errorf("bob, who has no budget, was held to one: %+v", refusal)
	}
}

// A server that keeps people's sign-ins to apps and holds no credential has no gateway to give them to, and still starts.
func TestAServerWithConnectorsAndNoCredentialStarts(t *testing.T) {
	t.Setenv("GUSTO_CREDENTIALS", "")
	t.Setenv("GUSTO_MODEL_UPSTREAM", "")
	t.Setenv("GUSTO_CONNECTORS", t.TempDir())
	t.Setenv("GUSTO_PUBLIC_URL", "http://acme.test")
	s := &server{cfg: fromEnv(t)}
	index, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.index = index
	if err := s.openWayOut(); err != nil {
		t.Fatal(err)
	}
	if s.connect == nil || s.way.gateways != nil {
		t.Fatalf("connectors %v, gateways %v", s.connect, s.way.gateways)
	}
}
