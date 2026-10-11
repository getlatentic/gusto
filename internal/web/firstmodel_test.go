package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/registry"
)

// ownComputer is haas on a person's own computer: signed in by its own links, with a registry of the person's and no model yet.
func ownComputer(t *testing.T) (*tokened, *registry.Registry) {
	t.Helper()
	here := everything(t)
	reg, err := registry.Open(t.TempDir(), registry.Platform{}, registry.Rules{PrivateAddresses: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	var offered atomic.Pointer[Models]
	set := func(e registry.Effective) { m := Models(e.Models); offered.Store(&m) }
	set(reg.Effective())
	reg.OnChange(set)
	here.server.UseRegistry(reg, func() Models { return *offered.Load() })
	here.server.UseLocalSignIn("a-secret", "ada")
	return here, reg
}

// modelLister is a provider that lists two models to the key it was given, and refuses any other.
func modelLister(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer sk-theirs" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "small"}, {"id": "large"}}})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestAPersonAddsTheirFirstModelInOneStepAndEveryEngineIsOfferedIt(t *testing.T) {
	here, reg := ownComputer(t)
	admin := here.as(t, boss)
	if page := admin.ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, `action="/setup/model"`) || strings.Contains(page, `name="said"`) {
		t.Fatalf("the chat page does not ask for a model first:\n%s", page)
	}
	provider := modelLister(t)
	listed := admin.ask(t, "POST", "/setup/models", url.Values{"address": {provider.URL}, "key": {"sk-theirs"}})
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"models":["large","small"]`) {
		t.Errorf("the provider's models were not listed: %d %s", listed.Code, listed.Body)
	}
	if refused := admin.ask(t, "POST", "/setup/models", url.Values{"address": {provider.URL}, "key": {"wrong"}}); refused.Code != http.StatusBadGateway || !strings.Contains(refused.Body.String(), "check the key") {
		t.Errorf("a wrong key was not said: %d %s", refused.Code, refused.Body)
	}

	added := admin.ask(t, "POST", "/setup/model", url.Values{"address": {provider.URL}, "key": {"sk-theirs"}, "models": {"small, large"}})
	if added.Code != http.StatusSeeOther || strings.Contains(added.Header().Get("Location"), "refused") {
		t.Fatalf("the model was not added: %d %s", added.Code, added.Header().Get("Location"))
	}
	effective := reg.Effective()
	if !slices.Equal(effective.Models, []string{"local/small", "local/large"}) {
		t.Errorf("the models are not on: %v", effective.Models)
	}
	if held := effective.Credentials["local"]; held.Key != "sk-theirs" || !slices.Equal(held.Wires, []string{"chat"}) {
		t.Errorf("the provider is not held with its key and the shape it answers: %+v", held)
	}
	page := admin.ask(t, "GET", "/chat", nil).Body.String()
	if strings.Contains(page, `action="/setup/model"`) || !strings.Contains(page, `name="said"`) || strings.Contains(page, "sk-theirs") {
		t.Errorf("the chat page does not start a chat on the model, or shows the key:\n%s", page)
	}
}

func TestAModelIsAddedOnlyOnAPersonsOwnHaasAndOnlyWithAModelNamed(t *testing.T) {
	shared, _, _ := registered(t, registry.Rules{})
	if got := shared.as(t, boss).ask(t, "POST", "/setup/model", url.Values{"address": {"https://api.theirs.example"}, "models": {"x"}}); got.Code != http.StatusNotFound {
		t.Errorf("a shared server took a model in one step: %d", got.Code)
	}
	here, _ := ownComputer(t)
	got := here.as(t, boss).ask(t, "POST", "/setup/model", url.Values{"address": {"https://api.theirs.example"}, "key": {"k"}})
	if !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a provider with no model named was added: %s", got.Header().Get("Location"))
	}
}

func TestAProviderIsNamedAfterWhereItAnswers(t *testing.T) {
	here, reg := ownComputer(t)
	for address, want := range map[string]string{
		"https://api.deepseek.com": "deepseek", "https://api.anthropic.com": "anthropic", "https://openrouter.ai/api": "openrouter",
		"http://127.0.0.1:11434": "local", "http://localhost:1234": "local",
	} {
		if got, err := here.server.providerNameFor(address); err != nil || got != want {
			t.Errorf("%s is called %q, want %q (%v)", address, got, want, err)
		}
	}
	if err := reg.AddProvider(registry.Give{Name: "deepseek", Address: "https://api.deepseek.com", Reference: "stored", Key: "k"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := here.server.providerNameFor("https://api.deepseek.com"); got != "deepseek-2" {
		t.Errorf("a second provider at the same place is called %q", got)
	}
}

func TestOpenAIsOwnAnswersResponsesAndAnyOtherIsTranslatedForCodex(t *testing.T) {
	for address, want := range map[string][]string{"https://api.openai.com": {"chat", "responses"}, "https://api.deepseek.com": {"chat"}} {
		if got := wiresOf(address, proxy.Bearer); !slices.Equal(got, want) {
			t.Errorf("%s answers %v, want %v", address, got, want)
		}
	}
	if got := wiresOf("https://api.anthropic.com", proxy.Anthropic); got != nil {
		t.Errorf("an Anthropic-compatible provider answers %v", got)
	}
}
