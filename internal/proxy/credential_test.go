package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/spend"
)

// held is a proxy with several credentials, each pointing at its own stand-in
// so a call can be traced to the one that served it.
func held(t *testing.T, credentials Credentials) (*Proxy, *Tokens, map[string]*http.Header, string) {
	t.Helper()
	seen := map[string]*http.Header{}
	for name := range credentials {
		var headers http.Header
		seen[name] = &headers
		credential := credentials[name]
		credential.Name = name
		credential.Address = upstream(t, seen[name]).URL
		credentials[name] = credential
	}
	kept := t.TempDir()
	tokens := NewTokens()
	proxy := New(tokens, credentials,
		func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true },
		spend.NewLedger(spend.Policy{}), Allowlist{})
	return proxy, tokens, seen, kept
}

// A model names the credential that serves it, and the call reaches that one
// rather than whichever happens to be first.
func TestAModelNamesTheCredentialThatServesIt(t *testing.T) {
	proxy, tokens, seen, _ := held(t, Credentials{
		"anthropic": {Key: "the-anthropic-key", Auth: Anthropic},
		"platform":  {Key: "the-platform-key"},
	})
	token, err := tokens.Mint("a-job", "platform")
	if err != nil {
		t.Fatal(err)
	}
	if answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"gpt-oss"}`); answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	if got := seen["platform"].Get("Authorization"); got != "Bearer the-platform-key" {
		t.Errorf("the platform credential was sent as %q", got)
	}
	if len(*seen["anthropic"]) != 0 {
		t.Error("the call reached a credential the job was not given")
	}
}

// Anthropic does not take a bearer token, so the credential decides how it is
// presented — and the other header is cleared either way.
func TestACredentialIsPresentedTheWayItsProviderTakesIt(t *testing.T) {
	proxy, tokens, seen, _ := held(t, Credentials{
		"anthropic": {Key: "the-anthropic-key", Auth: Anthropic},
	})
	token, _ := tokens.Mint("a-job", "anthropic")
	if answer := ask(t, proxy, "/model/"+token+"/v1/messages", `{"model":"claude"}`); answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	sent := seen["anthropic"]
	if got := sent.Get("x-api-key"); got != "the-anthropic-key" {
		t.Errorf("the key was sent as %q", got)
	}
	if got := sent.Get("anthropic-version"); got != AnthropicVersion {
		t.Errorf("the version was sent as %q", got)
	}
	if got := sent.Get("Authorization"); got != "" {
		t.Errorf("the container's Authorization reached the provider as %q", got)
	}
}

// The credential is decided when the job starts and carried by its token, so a
// container naming another provider's model in the body does not reach it.
func TestAContainerCannotChangeCredentialByNamingAnotherModel(t *testing.T) {
	proxy, tokens, seen, _ := held(t, Credentials{
		"anthropic": {Key: "the-anthropic-key", Auth: Anthropic},
		"platform":  {Key: "the-platform-key"},
	})
	token, _ := tokens.Mint("a-job", "platform")
	ask(t, proxy, "/model/"+token+"/v1/messages", `{"model":"anthropic/claude-opus-5-5"}`)
	if len(*seen["anthropic"]) != 0 {
		t.Fatal("naming another credential's model in the body reached that credential")
	}
	if got := seen["platform"].Get("Authorization"); got != "Bearer the-platform-key" {
		t.Errorf("the job's own credential was not used: %q", got)
	}
}

// What was spent is attributed to the credential that served it, so a period
// reconciles against that provider's own invoice.
func TestSpendIsRecordedAgainstTheCredentialThatServedIt(t *testing.T) {
	proxy, tokens, _, kept := held(t, Credentials{"platform": {Key: "k"}})
	token, _ := tokens.Mint("a-job", "platform")
	ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"gpt-oss"}`)

	text, err := os.ReadFile(filepath.Join(kept, "a-job.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var call struct {
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal([]byte(strings.SplitN(string(text), "\n", 2)[0]), &call); err != nil {
		t.Fatal(err)
	}
	if call.Credential != "platform" {
		t.Errorf("the call was recorded against %q", call.Credential)
	}
}

func TestAModelSaysWhichCredentialServesIt(t *testing.T) {
	for _, each := range []struct{ model, credential, rest string }{
		{"platform/qwen.qwen3-vl-235b-a22b-instruct", "platform", "qwen.qwen3-vl-235b-a22b-instruct"},
		{"anthropic/claude-opus-5-5", "anthropic", "claude-opus-5-5"},
		{"gpt-oss:20b", "", "gpt-oss:20b"},
		{"", "", ""},
	} {
		credential, rest := Named(each.model)
		if credential != each.credential || rest != each.rest {
			t.Errorf("%q split into %q and %q", each.model, credential, rest)
		}
	}
}

// A deployment that names its credentials is strict; one configured the older
// way holds a single unnamed credential that serves everything.
func TestANameThisDeploymentDoesNotHoldIsNotQuietlyServed(t *testing.T) {
	named := Credentials{"platform": {Name: "platform"}}
	if _, found := named.Get("anthropic"); found {
		t.Error("a credential nobody holds was served by another")
	}
	if _, found := named.Get("platform"); !found {
		t.Error("a credential it holds was not found")
	}

	older := Credentials{"": {Name: "the platform credential"}}
	for _, asked := range []string{"", "platform", "anything"} {
		if _, found := older.Get(asked); !found {
			t.Errorf("the single unnamed credential did not serve %q", asked)
		}
	}
}

func TestCredentialsAreReadFromADirectoryAndNamedByTheirFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("platform.json", `{"address":"https://example.invalid","key":"k"}`)
	write("anthropic.json", `{"address":"https://api.anthropic.com","key":"k2","auth":"anthropic"}`)

	credentials, err := OpenCredentials(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := credentials.Names(); len(got) != 2 || got[0] != "anthropic" || got[1] != "platform" {
		t.Fatalf("read %v", got)
	}
	if held, _ := credentials.Get("anthropic"); held.Auth != Anthropic || held.Name != "anthropic" {
		t.Errorf("read %+v", held)
	}
}

func TestACredentialWithNoAddressIsRefusedByName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"key":"k"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := OpenCredentials(dir)
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("a credential with no address gave %v", err)
	}
}

// A container is told one address and puts its token where its own provider's
// key belongs. An Anthropic-shaped client puts it in x-api-key, and the proxy
// answered those with "no": Claude Code read that as a failed sign-in and went
// to api.anthropic.com instead, where the allowlist stopped it.
func TestATokenIsTakenFromWhereeverTheClientPutsIt(t *testing.T) {
	proxy, tokens, seen, _ := held(t, Credentials{"anthropic": {Key: "the-real-key", Auth: Anthropic}})
	token, err := tokens.Mint("a-job", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	for _, where := range []struct{ header, value string }{
		{"Authorization", "Bearer " + token},
		{"x-api-key", token},
	} {
		request := httptest.NewRequest("POST", "/model/v1/messages", strings.NewReader(`{"model":"claude"}`))
		request.Header.Set(where.header, where.value)
		answer := httptest.NewRecorder()
		proxy.Handler().ServeHTTP(answer, request)
		if answer.Code != http.StatusOK {
			t.Errorf("a token in %s was answered %d", where.header, answer.Code)
		}
	}
	if got := seen["anthropic"].Get("x-api-key"); got != "the-real-key" {
		t.Errorf("the credential reached the provider as %q", got)
	}
}
