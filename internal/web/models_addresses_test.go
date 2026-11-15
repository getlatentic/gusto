package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/pkg/proxy"
)

func TestAnAdminGivesOneProviderAnOpenAIAndAnAnthropicAddressUnderOneKey(t *testing.T) {
	here, reg, _ := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{
		"name": {"acme"}, "address": {"https://api.theirs.example"}, "kind": {"openai"}, "source": {"stored"}, "key": {theirKey},
	})
	added := admin.ask(t, "POST", "/admin/models/providers/acme/addresses", url.Values{"address": {"https://api.theirs.example/anthropic"}, "kind": {"anthropic"}})
	if added.Code != http.StatusSeeOther || added.Header().Get("Location") != providerPage("acme") {
		t.Fatalf("%d %v", added.Code, added.Header())
	}
	admin.ask(t, "POST", "/admin/models/providers/acme/enable", nil)
	held := reg.Effective().Credentials["acme"]
	if held.Address != "https://api.theirs.example" || held.Auth != proxy.Bearer || len(held.Endpoints) != 1 ||
		held.Endpoints[0].Address != "https://api.theirs.example/anthropic" || held.Endpoints[0].Auth != proxy.Anthropic {
		t.Fatalf("the proxy holds %+v", held)
	}
	acme := modelsAt(t, admin, added.Header().Get("Location")).Provider
	if acme == nil || len(acme.Addresses) != 2 || acme.Addresses[0].Kind != "OpenAI-compatible" || acme.Addresses[1].Kind != "Anthropic-compatible" || !acme.Addresses[1].Extra {
		t.Errorf("the page does not give both addresses and their kinds: %+v", acme)
	}
	admin.ask(t, "POST", "/admin/models/providers/acme/addresses/remove", url.Values{"address": {"https://api.theirs.example/anthropic"}})
	if len(reg.Effective().Credentials["acme"].Endpoints) != 0 {
		t.Error("the address is still held after it was removed")
	}
	kinds := map[string]int{}
	for _, event := range written(t, here.log) {
		kinds[event.Kind]++
	}
	if kinds[changelog.AddressAdded] != 1 || kinds[changelog.AddressRemoved] != 1 {
		t.Errorf("each change is recorded: %v", kinds)
	}
}

func TestACustomAddressNamesTheShapesItAnswers(t *testing.T) {
	here, reg, _ := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	refused := admin.ask(t, "POST", "/admin/models/providers", url.Values{
		"name": {"acme"}, "address": {"https://api.theirs.example"}, "kind": {"custom"}, "auth": {"bearer"}, "source": {"stored"}, "key": {theirKey},
	})
	if !strings.Contains(refused.Header().Get("Location"), "refused") {
		t.Fatal("a custom address that answers nothing was kept")
	}
	admin.ask(t, "POST", "/admin/models/providers", url.Values{
		"name": {"acme"}, "address": {"https://api.theirs.example"}, "kind": {"custom"}, "auth": {"bearer"}, "wires": {"chat"}, "source": {"stored"}, "key": {theirKey},
	})
	if provider := reg.Document().Providers[0]; len(provider.Wires) != 1 || provider.Wires[0] != "chat" {
		t.Fatalf("the provider is %+v", provider)
	}
	if acme := modelsAt(t, admin, providerPage("acme")).Provider; acme == nil || acme.Addresses[0].Kind != "custom: chat, bearer token" {
		t.Errorf("the page does not say the address is custom and what it answers: %+v", acme)
	}
}
