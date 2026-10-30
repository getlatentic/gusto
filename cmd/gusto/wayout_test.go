package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/job"
)

// A sealed box reaches its model only through the proxy, on its own loopback,
// so the runner inside cannot tell where the code goes. The server can: the
// credential names the provider. Restricted code bound for a provider outside
// the building is refused before a box starts, and says so where the job is
// read.
func TestRestrictedCodeIsRefusedACredentialOutsideTheBuilding(t *testing.T) {
	credentials := t.TempDir()
	os.WriteFile(filepath.Join(credentials, "deepseek.json"),
		[]byte(`{"address": "https://api.deepseek.com", "key": "sk-test"}`), 0o600)
	t.Setenv("GUSTO_CREDENTIALS", credentials)
	way, err := openWayOut(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "b", Harness: "opencode", Model: "deepseek/deepseek-flash", Sensitivity: job.Restricted}
	if _, err := way.Run(dir, spec); err == nil || !strings.Contains(err.Error(), "api.deepseek.com") {
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
	way, err := openWayOut(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if way.InBuilding() {
		t.Error("a credential with an address outside was said to be inside the building")
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "b", Harness: "claude", Model: "inside/model", Sensitivity: job.Restricted}
	if _, err := way.Run(dir, spec); err == nil || !strings.Contains(err.Error(), "api.deepseek.com") {
		t.Fatalf("restricted code was not refused the second address: %v", err)
	}
}

// A harness that needs no model has no credential to name, and a deployment that holds credentials still runs it, as
// the install check's first job does. A harness that does need one is still asked to name it.
func TestAHarnessThatNeedsNoModelIsNotRefusedForWantOfACredential(t *testing.T) {
	harnesses := t.TempDir()
	os.WriteFile(filepath.Join(harnesses, "plain.json"), []byte(`{"name":"plain","program":"true","needs":"nothing","stream":"plain"}`), 0o600)
	t.Setenv("GUSTO_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if needsModel(dir, job.Spec{Harness: "plain"}) {
		t.Error("a harness that needs nothing was said to need a model")
	}
	if !needsModel(dir, job.Spec{Harness: "opencode"}) || !needsModel(dir, job.Spec{Harness: "nowhere"}) {
		t.Error("a harness that needs a model, or is not found, was said to need none")
	}
}

// A job run in the server's own process goes through the proxy as a box does: it is given a token for its credential and the proxy
// on loopback, and none of the server's environment that is its own.
func TestAJobRunHereIsGivenTheProxyAndATokenAndNoneOfTheServersSecrets(t *testing.T) {
	credentials := t.TempDir()
	os.WriteFile(filepath.Join(credentials, "deepseek.json"),
		[]byte(`{"address": "https://api.deepseek.com", "key": "sk-the-real-key"}`), 0o600)
	t.Setenv("GUSTO_CREDENTIALS", credentials)
	t.Setenv("GUSTO_SANDBOX", "here")
	t.Setenv("GUSTO_PROXY_ADDR", "")
	t.Setenv("GUSTO_PASSWORD", "the-servers-own-secret")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "an-aws-secret")
	way, err := openWayOut(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if way.sealed || way.address != "127.0.0.1:8041" {
		t.Errorf("the proxy is not on loopback for a job run here: sealed %v, %s", way.sealed, way.address)
	}
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	env, err := way.hereEnv(dir, job.Spec{Brief: "b", Harness: "opencode", Model: "deepseek/deepseek-flash"})
	if err != nil {
		t.Fatal(err)
	}
	if env.ModelToken == "" || env.ModelURL != "http://127.0.0.1:8041/model/v1" {
		t.Errorf("the job was not given the proxy and a token: %+v", env)
	}
	joined := strings.Join(env.Base, "\n")
	for _, secret := range []string{"the-servers-own-secret", "an-aws-secret", "sk-the-real-key", "GUSTO_CREDENTIALS"} {
		if strings.Contains(joined, secret) {
			t.Errorf("%s is in the job's environment", secret)
		}
	}
	for _, want := range []string{"PATH=", "HOME=", "GUSTO_MODEL_TOKEN=" + env.ModelToken} {
		if !strings.Contains(joined, want) {
			t.Errorf("the job's environment lacks %s", want)
		}
	}
	way.Done(dir)
	if _, err := way.hereEnv(dir, job.Spec{Brief: "b", Harness: "opencode", Model: "nowhere/model"}); err == nil {
		t.Error("a job naming a credential the server does not hold was given a token")
	}
}

func TestTheCallLimitsAreReadFromTheirSettingsAndAWrongOneIsSaid(t *testing.T) {
	t.Setenv("GUSTO_PROXY_MAX_OUTPUT_TOKENS", "16384")
	t.Setenv("GUSTO_PROXY_SILENCE_SECONDS", "60")
	limits, err := callLimits()
	if err != nil || limits.MaxOutput != 16384 || limits.Silence != time.Minute {
		t.Fatalf("%+v %v", limits, err)
	}
	t.Setenv("GUSTO_PROXY_SILENCE_SECONDS", "a minute")
	if _, err := callLimits(); err == nil || !strings.Contains(err.Error(), "GUSTO_PROXY_SILENCE_SECONDS") {
		t.Errorf("a wrong setting was taken: %v", err)
	}
}

func TestATaskIsHeldToTheBudgetOfThePersonWhoAskedForIt(t *testing.T) {
	way := &WayOut{}
	way.UseMemberBudgets(func(person string) (float64, bool) { return 20, person == "ada@acme.com" })
	if terms := way.termsOf(job.Spec{Person: "Ada@Acme.com"}); terms.PersonUSD == nil || *terms.PersonUSD != 20 || terms.Person != "ada@acme.com" {
		t.Errorf("ada's task: %+v", terms)
	}
	if terms := way.termsOf(job.Spec{Person: "bob@acme.com"}); terms.PersonUSD != nil {
		t.Errorf("bob, who has no budget, was given one: %+v", terms)
	}
}
