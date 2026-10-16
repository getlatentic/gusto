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

	"errors"
	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/proxy"
	"github.com/getlatentic/gusto/internal/registry"
)

// ownComputer is gusto on a person's own computer: signed in by its own links, with a registry of the person's and no model yet.
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

func TestAModelIsAddedOnlyOnAPersonsOwnGustoAndOnlyWithAModelNamed(t *testing.T) {
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

// A model named through a provider that does not offer it, when another does, is told apart by its whole name.
func TestAModelOfferedThroughAnotherProviderIsNamedWhole(t *testing.T) {
	here, reg := ownComputer(t)
	if err := reg.AddProvider(registry.Give{Name: "host", Address: "http://127.0.0.1:9", Reference: "stored", Key: "k"}); err != nil {
		t.Fatal(err)
	}
	reg.Enable("host", "")
	reg.AddModel("host", "fake-small", nil)
	reg.Enable("host", "fake-small")
	got := here.as(t, boss).ask(t, "POST", "/chat", url.Values{"said": {"hi"}, "harness": {"fixture"}, "model": {"local/fake-small"}})
	where, _ := url.Parse(got.Header().Get("Location"))
	if refused := where.Query().Get("refused"); refused != "local/fake-small is not offered here: choose host/fake-small" {
		t.Errorf("refused with %q", refused)
	}
}

// A computer that lacks what gusto needs is told how to get it, and no agent is started until it has it.
func TestWhatTheComputerLacksIsSaidAndNoAgentStartsWithoutIt(t *testing.T) {
	here, _ := ownComputer(t)
	lacking := []string{"gusto needs Git, which keeps each conversation's files: install it"}
	here.server.UseNeeds(func() []string { return lacking })
	admin := here.as(t, boss)
	if page := admin.ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, "gusto needs Git") {
		t.Error("the chat page does not say what the computer lacks")
	}
	got := admin.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}})
	if where, _ := url.Parse(got.Header().Get("Location")); where.Query().Get("refused") != lacking[0] {
		t.Errorf("an agent was started on a computer that lacks git: %s", got.Header().Get("Location"))
	}
}

// A computer with no wall is said to have none on the chat page, and no agent starts until the person has confirmed, once.
func TestAnAgentWaitsForAPersonWithNoSandboxToConfirmOnce(t *testing.T) {
	here, _ := ownComputer(t)
	if err := ownModel(t, here); err != nil {
		t.Fatal(err)
	}
	var confirmed atomic.Bool
	here.server.UseWall(Wall{Choice: isolation.Choice{Kind: isolation.Nothing, Passed: []isolation.Backend{{Kind: isolation.Container, Detail: "neither Docker nor Podman is installed"}}},
		Accepted: confirmed.Load, Accept: func() error { confirmed.Store(true); return nil }})
	admin := here.as(t, boss)
	page := admin.ask(t, "GET", "/chat", nil).Body.String()
	if !strings.Contains(page, `action="/setup/no-sandbox"`) || !strings.Contains(page, "Nothing stands between an agent and your files") || strings.Contains(page, `name="said"`) {
		t.Errorf("the chat page does not hold the first agent for a confirmation:\n%s", page)
	}
	if got := admin.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}}); !strings.Contains(got.Header().Get("Location"), "refused=confirm") {
		t.Errorf("an agent started before the person confirmed: %s", got.Header().Get("Location"))
	}
	if got := admin.ask(t, "POST", "/setup/no-sandbox", nil); got.Code != http.StatusSeeOther || !confirmed.Load() {
		t.Fatalf("the confirmation was not kept: %d", got.Code)
	}
	page = admin.ask(t, "GET", "/chat", nil).Body.String()
	if strings.Contains(page, `action="/setup/no-sandbox"`) || !strings.Contains(page, "Agents run in no sandbox.") || !strings.Contains(page, "neither Docker nor Podman is installed") {
		t.Errorf("after confirming, the page does not say which wall and why no stronger:\n%s", page)
	}
	if got := admin.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}}); !strings.Contains(got.Header().Get("Location"), "/threads/") {
		t.Errorf("an agent did not start after the confirmation: %s", got.Header().Get("Location"))
	}
}

// A computer with a wall is told no confirmation, and one cannot be given.
func TestAWallNeedsNoConfirmationAndNoneCanBeGiven(t *testing.T) {
	here, _ := ownComputer(t)
	if err := ownModel(t, here); err != nil {
		t.Fatal(err)
	}
	here.server.UseWall(Wall{Choice: isolation.Choice{Kind: isolation.Container, Detail: "docker"}, Accepted: func() bool { return false }, Accept: func() error { return nil }})
	admin := here.as(t, boss)
	page := admin.ask(t, "GET", "/chat", nil).Body.String()
	if strings.Contains(page, "no-sandbox") || !strings.Contains(page, "Agents run in a container.") {
		t.Errorf("the page does not say the wall:\n%s", page)
	}
	if got := admin.ask(t, "POST", "/setup/no-sandbox", nil); got.Code != http.StatusNotFound {
		t.Errorf("a confirmation was taken for a computer that has a wall: %d", got.Code)
	}
}

// ownModel gives a person's own gusto the one model its chat page needs to show a box.
func ownModel(t *testing.T, here *tokened) error {
	t.Helper()
	form := url.Values{"address": {"https://api.theirs.example"}, "key": {"k"}, "models": {"small"}}
	if got := here.as(t, boss).ask(t, "POST", "/setup/model", form); strings.Contains(got.Header().Get("Location"), "refused") {
		return errors.New(got.Header().Get("Location"))
	}
	return nil
}
