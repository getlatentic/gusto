package web

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/registry"
	"github.com/getlatentic/haas/internal/spend"
)

// registered is a console whose organisation has a registry, and the models the forms offer as that registry comes to.
func registered(t *testing.T, rules registry.Rules) (*tokened, *registry.Registry, *atomic.Pointer[Models]) {
	t.Helper()
	here := everything(t)
	platform := registry.Platform{
		Credentials: proxy.Credentials{"platform": {Name: "platform", Address: "https://models.operator.example", Key: "the-operators-key"}},
		Models:      []string{"platform/qwen"},
		Prices:      map[string]spend.Price{"platform/qwen": {InputPerMillion: 1, OutputPerMillion: 2}},
	}
	rules.Harnesses = []string{"fixture", "claude"}
	reg, err := registry.Open(t.TempDir(), platform, rules, nil)
	if err != nil {
		t.Fatal(err)
	}
	reg.SetResolver(func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "api.theirs.example" {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		return nil, errors.New("no such host")
	})
	var offered atomic.Pointer[Models]
	set := func(e registry.Effective) { m := Models{"": e.Models}; offered.Store(&m) }
	set(reg.Effective())
	reg.OnChange(set)
	here.server.UseRegistry(reg, func() Models { return *offered.Load() })
	return here, reg, &offered
}

const theirKey = "sk-their-very-secret-key"

func TestAnAdminAddsAProviderAndAModelAndTheFormsOfferItWithNoKeyInAnyPageOrRecord(t *testing.T) {
	here, reg, offered := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	first := admin.ask(t, "POST", "/admin/models/providers", url.Values{
		"name": {"acme"}, "address": {"https://api.theirs.example/v1"}, "source": {"stored"}, "key": {theirKey},
	})
	if first.Code != http.StatusSeeOther || first.Header().Get("Location") != "/admin/models" {
		t.Fatalf("%d %v", first.Code, first.Header())
	}
	added := admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big-1"}, "input": {"0.5"}, "output": {"1.5"}})
	if added.Code != http.StatusSeeOther || strings.Contains(added.Header().Get("Location"), "refused") {
		t.Fatalf("%d %v", added.Code, added.Header())
	}
	if got := (*offered.Load())[""]; len(got) != 2 || got[1] != "acme/big-1" {
		t.Errorf("the forms offer what the registry comes to: %v", got)
	}
	if !here.server.offered().Offers("fixture", "acme/big-1") || here.server.offered().Offers("fixture", "acme/other") {
		t.Error("a page asks what is offered when it is drawn, and a model nobody added is not")
	}
	page := admin.ask(t, "GET", "/admin/models", nil)
	body := page.Body.String()
	for _, want := range []string{"acme/big-1", "platform/qwen", "set by the operator", "$0.5", "$1.5", "Registry version 2"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not say %q", want)
		}
	}
	if strings.Contains(body, theirKey) || strings.Contains(body, "the-operators-key") {
		t.Error("a key is in a page")
	}
	if !strings.Contains(body, `href="/admin/models"`) {
		t.Error("the console does not list the page")
	}
	for _, event := range written(t, here.log) {
		for _, value := range event.What {
			if strings.Contains(value, theirKey) {
				t.Errorf("a key is in the record of changes: %+v", event)
			}
		}
	}
	kinds := map[string]int{}
	for _, event := range written(t, here.log) {
		kinds[event.Kind]++
	}
	if kinds[changelog.ProviderAdded] != 1 || kinds[changelog.ModelAdded] != 1 {
		t.Errorf("each change is recorded: %v", kinds)
	}
	if reg.Document().Version != 2 {
		t.Error("two changes are two versions")
	}
}

func TestAnAdminChangesPricesRetiresAndReplacesAKeyThroughThePage(t *testing.T) {
	here, reg, offered := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"org/big-1"}})
	if got := admin.ask(t, "POST", "/admin/models/providers/acme/price", url.Values{"model": {"org/big-1"}, "input": {"3"}, "output": {"4"}, "cached": {"0.3"}}); strings.Contains(got.Header().Get("Location"), "refused") {
		t.Fatalf("%v", got.Header())
	}
	if price := reg.Effective().Prices["acme/org/big-1"]; price.OutputPerMillion != 4 || price.CachedInputPerMillion == nil || *price.CachedInputPerMillion != 0.3 {
		t.Errorf("a model whose name has a slash in it is priced: %+v", price)
	}
	admin.ask(t, "POST", "/admin/models/providers/acme/key", url.Values{"key": {"sk-another"}})
	if reg.Effective().Credentials["acme"].Key != "sk-another" {
		t.Error("the key was not replaced")
	}
	admin.ask(t, "POST", "/admin/models/providers/acme/models/retire", url.Values{"model": {"org/big-1"}})
	if got := (*offered.Load())[""]; len(got) != 1 {
		t.Errorf("a retired model is no longer offered: %v", got)
	}
	admin.ask(t, "POST", "/admin/models/providers/acme/retire", nil)
	if _, held := reg.Effective().Credentials["acme"]; held {
		t.Error("a retired provider is still held")
	}
	if body := admin.ask(t, "GET", "/admin/models", nil).Body.String(); strings.Contains(body, "https://api.theirs.example") {
		t.Error("a retired provider is still on the page")
	}
}

func TestWhatTheRegistryRefusesIsSaidOnThePageAndNothingIsChanged(t *testing.T) {
	here, reg, _ := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	for name, form := range map[string]url.Values{
		"a private address":       {"name": {"x"}, "address": {"https://10.0.0.5"}, "source": {"stored"}, "key": {"k"}},
		"a file on this machine":  {"name": {"x"}, "address": {"https://api.theirs.example"}, "source": {"file"}, "reference": {"/etc/passwd"}},
		"the operator's name":     {"name": {"platform"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {"k"}},
		"no key":                  {"name": {"x"}, "address": {"https://api.theirs.example"}, "source": {"stored"}},
		"plain http":              {"name": {"x"}, "address": {"http://api.theirs.example"}, "source": {"stored"}, "key": {"k"}},
		"a harness there is none": {"name": {"x"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {"k"}, "harness": {"emacs"}},
	} {
		got := admin.ask(t, "POST", "/admin/models/providers", form)
		if got.Code != http.StatusSeeOther || !strings.HasPrefix(got.Header().Get("Location"), "/admin/models?refused=") {
			t.Errorf("%s: %d %v", name, got.Code, got.Header())
		}
	}
	if reg.Document().Version != 0 {
		t.Errorf("a refused change was made: %+v", reg.Document())
	}
	shown := admin.ask(t, "GET", "/admin/models?refused="+url.QueryEscape("a provider is reached over https"), nil).Body.String()
	if !strings.Contains(shown, "a provider is reached over https") {
		t.Error("the reason is not on the page")
	}
	if got := admin.ask(t, "POST", "/admin/models/providers/nobody/models", url.Values{"model": {"m"}}); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a model for a provider that is not there: %v", got.Header())
	}
	if got := admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"m"}, "input": {"1"}}); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a price with no output: %v", got.Header())
	}
	if got := admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"m"}, "input": {"x"}, "output": {"1"}}); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a price that is not a number: %v", got.Header())
	}
}

func TestOnlyAnAdminReachesTheModelsPageAndAServerWithNoRegistryHasNoPage(t *testing.T) {
	here, reg, _ := registered(t, registry.Rules{})
	runner := here.as(t, alice)
	stranger := here.site
	form := url.Values{"name": {"x"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {"k"}}
	for _, path := range []string{"/admin/models/providers", "/admin/models/providers/acme/retire", "/admin/models/providers/acme/key", "/admin/models/providers/acme/models", "/admin/models/providers/acme/price", "/admin/models/providers/acme/models/retire"} {
		if got := runner.ask(t, "POST", path, form); got.Code != http.StatusForbidden {
			t.Errorf("a runner posting to %s got %d", path, got.Code)
		}
		if got := stranger.ask(t, "POST", path, form); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/login" {
			t.Errorf("a stranger posting to %s: %d", path, got.Code)
		}
	}
	if got := runner.ask(t, "GET", "/admin/models", nil); got.Code != http.StatusForbidden {
		t.Errorf("a runner reading it: %d", got.Code)
	}
	if reg.Document().Version != 0 {
		t.Error("somebody who is not an admin changed the registry")
	}
	bare := everything(t)
	if got := bare.as(t, boss).ask(t, "GET", "/admin/models", nil); got.Code != http.StatusNotFound {
		t.Errorf("a server with no registry has a page of it: %d", got.Code)
	}
	if strings.Contains(bare.as(t, boss).ask(t, "GET", "/admin", nil).Body.String(), `href="/admin/models"`) {
		t.Error("the console lists a page there is not")
	}
}
