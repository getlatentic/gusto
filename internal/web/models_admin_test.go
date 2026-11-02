package web

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
)

// registered is a console whose organization has a registry, and the models the forms offer as that registry comes to.
func registered(t *testing.T, rules registry.Rules) (*tokened, *registry.Registry, *atomic.Pointer[Models]) {
	t.Helper()
	return registeredAs(t, rules, false)
}

func registeredAs(t *testing.T, rules registry.Rules, editable bool) (*tokened, *registry.Registry, *atomic.Pointer[Models]) {
	t.Helper()
	here := everything(t)
	platform := registry.Platform{
		Credentials: proxy.Credentials{"platform": {Name: "platform", Address: "https://models.operator.example", Key: "the-operators-key"}},
		Models:      []string{"platform/qwen"},
		Prices:      map[string]spend.Price{"platform/qwen": {InputPerMillion: 1, OutputPerMillion: 2}},
	}
	reg, err := registry.Open(t.TempDir(), platform, rules, editable)
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
	set := func(e registry.Effective) { m := Models(e.Models); offered.Store(&m) }
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
	if first.Code != http.StatusSeeOther || first.Header().Get("Location") != modelsTab("providers") {
		t.Fatalf("%d %v", first.Code, first.Header())
	}
	added := admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big-1"}, "input": {"0.5"}, "output": {"1.5"}})
	if added.Code != http.StatusSeeOther || added.Header().Get("Location") != providerPage("acme") {
		t.Fatalf("%d %v", added.Code, added.Header())
	}
	if got := *offered.Load(); len(got) != 1 {
		t.Errorf("what was only added is offered before it was turned on: %v", got)
	}
	if body := admin.ask(t, "GET", providerPage("acme"), nil).Body.String(); !strings.Contains(body, "waiting for you to turn it on") || !strings.Contains(body, "Turn on acme") {
		t.Error("the page does not say what is waiting")
	}
	admin.ask(t, "POST", "/admin/models/providers/acme/enable", nil)
	admin.ask(t, "POST", "/admin/models/providers/acme/models/enable", url.Values{"model": {"big-1"}})
	if got := *offered.Load(); len(got) != 2 || got[1] != "acme/big-1" {
		t.Errorf("the forms offer what the registry comes to: %v", got)
	}
	if !here.server.offered().Offers("acme/big-1") || here.server.offered().Offers("acme/other") {
		t.Error("a page asks what is offered when it is drawn, and a model nobody added is not")
	}
	for path, wants := range map[string][]string{
		modelsTab("models"):      {"acme/big-1", "platform/qwen", `href="/admin/models"`},
		modelsTab("providers"):   {"set by the operator", `href="/admin/models?tab=providers&amp;provider=acme"`},
		providerPage("acme"):     {"acme/big-1", `value="0.5"`, `value="1.5"`},
		providerPage("platform"): {"platform/qwen", "set by the operator of this gusto"},
		modelsTab("access"):      {"Limit a person"},
		modelsTab("settings"):    {"Version 3", "Go back to it"},
	} {
		body := admin.ask(t, "GET", path, nil).Body.String()
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("%s does not say %q", path, want)
			}
		}
		if strings.Contains(body, theirKey) || strings.Contains(body, "the-operators-key") {
			t.Errorf("a key is in %s", path)
		}
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
	if kinds[changelog.ProviderAdded] != 1 || kinds[changelog.ModelAdded] != 1 || kinds[changelog.ProviderEnabled] != 1 || kinds[changelog.ModelEnabled] != 1 {
		t.Errorf("each change is recorded: %v", kinds)
	}
	for _, event := range written(t, here.log) {
		if event.Kind == changelog.ModelEnabled && (event.What["was"] != "off" || event.What["now"] != "on" || event.What["model"] != "acme/big-1") {
			t.Errorf("a turning on does not say what it was and became: %+v", event.What)
		}
	}
	if reg.Document().Version != 4 {
		t.Error("four changes are four versions")
	}
}

func TestAnAdminChangesPricesRetiresAndReplacesAKeyThroughThePage(t *testing.T) {
	here, reg, offered := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"org/big-1"}})
	admin.ask(t, "POST", "/admin/models/providers/acme/enable", nil)
	admin.ask(t, "POST", "/admin/models/providers/acme/models/enable", url.Values{"model": {"org/big-1"}})
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
	if got := *offered.Load(); len(got) != 1 {
		t.Errorf("a retired model is no longer offered: %v", got)
	}
	admin.ask(t, "POST", "/admin/models/providers/acme/retire", nil)
	if _, held := reg.Effective().Credentials["acme"]; held {
		t.Error("a retired provider is still held")
	}
	if body := admin.ask(t, "GET", modelsTab("providers"), nil).Body.String(); strings.Contains(body, "https://api.theirs.example") {
		t.Error("a retired provider is still on the page")
	}
}

func TestWhatTheRegistryRefusesIsSaidOnThePageAndNothingIsChanged(t *testing.T) {
	here, reg, _ := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	for name, form := range map[string]url.Values{
		"a private address":      {"name": {"x"}, "address": {"https://10.0.0.5"}, "source": {"stored"}, "key": {"k"}},
		"a file on this machine": {"name": {"x"}, "address": {"https://api.theirs.example"}, "source": {"file"}, "reference": {"/etc/passwd"}},
		"the operator's name":    {"name": {"platform"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {"k"}},
		"no key":                 {"name": {"x"}, "address": {"https://api.theirs.example"}, "source": {"stored"}},
		"plain http":             {"name": {"x"}, "address": {"http://api.theirs.example"}, "source": {"stored"}, "key": {"k"}},
	} {
		got := admin.ask(t, "POST", "/admin/models/providers", form)
		if got.Code != http.StatusSeeOther || !strings.HasPrefix(got.Header().Get("Location"), modelsTab("providers")+"&refused=") {
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

func TestAnAdminOfADeploymentThatIsTheirsChoosesWhatProvidersMayBeAndAHostedOneCannot(t *testing.T) {
	here, reg, _ := registeredAs(t, registry.Rules{}, true)
	admin := here.as(t, boss)
	page := admin.ask(t, "GET", modelsTab("providers"), nil).Body.String()
	if !strings.Contains(page, `action="/admin/models/policy"`) || strings.Contains(page, " checked") {
		t.Errorf("the form is there, with everything off:\n%s", page)
	}
	got := admin.ask(t, "POST", "/admin/models/policy", url.Values{"private": {"on"}, "keys": {"ssm", "file"}})
	if got.Code != http.StatusSeeOther || got.Header().Get("Location") != modelsTab("providers") {
		t.Fatalf("%d %v", got.Code, got.Header())
	}
	if rules := reg.Rules(); !rules.PrivateAddresses || len(rules.KeySources) != 2 {
		t.Errorf("%+v", rules)
	}
	page = admin.ask(t, "GET", modelsTab("providers"), nil).Body.String()
	if strings.Count(page, " checked") != 3 {
		t.Errorf("the form says what is chosen: %d checked", strings.Count(page, " checked"))
	}
	var said []string
	for _, event := range written(t, here.log) {
		if event.Kind == changelog.PolicySet {
			said = append(said, event.What["private"]+"/"+event.What["keys"])
		}
	}
	if len(said) != 1 || said[0] != "yes/file, ssm" {
		t.Errorf("the choice is in the record of changes: %v", said)
	}
	if bad := admin.ask(t, "POST", "/admin/models/policy", url.Values{"keys": {"env"}}); !strings.Contains(bad.Header().Get("Location"), "refused=") {
		t.Errorf("a kind that is not one: %v", bad.Header())
	}
	if runner := here.as(t, alice).ask(t, "POST", "/admin/models/policy", url.Values{"private": {"on"}}); runner.Code != http.StatusForbidden {
		t.Errorf("a runner chose: %d", runner.Code)
	}

	hosted, hostedReg, _ := registeredAs(t, registry.Rules{}, false)
	boss := hosted.as(t, boss)
	refused := boss.ask(t, "POST", "/admin/models/policy", url.Values{"private": {"on"}})
	if !strings.Contains(refused.Header().Get("Location"), "refused=") || hostedReg.Rules().PrivateAddresses {
		t.Errorf("a hosted admin loosened what keeps the machine safe: %v", refused.Header())
	}
	if hostedPage := boss.ask(t, "GET", modelsTab("providers"), nil).Body.String(); strings.Contains(hostedPage, "/admin/models/policy") || !strings.Contains(hostedPage, "operator fixes") {
		t.Error("a hosted page offers the choice")
	}
}

func TestALimitedPersonIsOfferedAndMayStartOnlyTheModelsTheyAreLimitedTo(t *testing.T) {
	here, reg, _ := registered(t, registry.Rules{})
	admin, ada, bob := here.as(t, boss), here.as(t, alice), here.as(t, bob)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big"}})
	admin.ask(t, "POST", "/admin/models/providers/acme/enable", nil)
	admin.ask(t, "POST", "/admin/models/providers/acme/models/enable", url.Values{"model": {"big"}})
	if got := admin.ask(t, "POST", "/admin/models/access", url.Values{"email": {alice}}); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a limit to nothing is refused, since it reads as no limit: %v", got.Header())
	}
	if got := admin.ask(t, "POST", "/admin/models/access", url.Values{"email": {alice}, "model": {"acme/big"}}); strings.Contains(got.Header().Get("Location"), "refused") {
		t.Fatalf("%v", got.Header())
	}
	if limits := reg.Limits(); len(limits) != 1 || limits[0].Email != alice {
		t.Fatalf("%+v", limits)
	}

	setUp := ada.ask(t, "GET", "/instances/new?harness=fixture", nil).Body.String()
	if !strings.Contains(setUp, "acme/big") || strings.Contains(setUp, "platform/qwen") {
		t.Errorf("a limited person is offered what they may use and no more:\n%s", setUp)
	}
	if bobs := bob.ask(t, "GET", "/instances/new?harness=fixture", nil).Body.String(); !strings.Contains(bobs, "platform/qwen") || !strings.Contains(bobs, "acme/big") {
		t.Error("somebody who is not limited is offered everything")
	}
	refused := ada.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "model": {"platform/qwen"}})
	if !strings.Contains(refused.Header().Get("Location"), "refused=") || len(here.server.instances.List()) != 0 {
		t.Errorf("a limited person started an agent on a model they may not use: %v", refused.Header())
	}
	allowed := ada.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "model": {"acme/big"}})
	if !strings.Contains(allowed.Header().Get("Location"), "/threads/") {
		t.Errorf("%v", allowed.Header())
	}
	blank := ada.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}})
	instance := strings.TrimPrefix(blank.Header().Get("Location"), "/instances/")
	if held, _ := here.server.instances.Get(instance[:strings.Index(instance, "/")]); held.Model != "acme/big" {
		t.Errorf("an agent started with no model is given one the person may use: %q", held.Model)
	}
	if bobOnQwen := bob.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "model": {"platform/qwen"}}); !strings.Contains(bobOnQwen.Header().Get("Location"), "/threads/") {
		t.Errorf("somebody who is not limited was refused: %v", bobOnQwen.Header())
	}

	admin.ask(t, "POST", "/admin/models/providers/acme/models/retire", url.Values{"model": {"big"}})
	if got := ada.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}}); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a person limited to models that are gone is not given the default of a deployment with none: %v", got.Header())
	}
	admin.ask(t, "POST", "/admin/models/access/clear", url.Values{"email": {alice}})
	if got := ada.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "model": {"platform/qwen"}}); !strings.Contains(got.Header().Get("Location"), "/threads/") {
		t.Errorf("a limit that is removed is gone: %v", got.Header())
	}
	var said []string
	for _, event := range written(t, here.log) {
		if event.Kind == changelog.AccessSet {
			said = append(said, event.What["email"]+" "+event.What["models"])
		}
	}
	if len(said) != 2 || said[0] != alice+" acme/big" || said[1] != alice+" " {
		t.Errorf("both changes are in the record: %q", said)
	}
	if runner := ada.ask(t, "POST", "/admin/models/access", url.Values{"email": {bob2()}, "model": {"platform/qwen"}}); runner.Code != http.StatusForbidden {
		t.Errorf("a runner limited somebody: %d", runner.Code)
	}
}

func bob2() string { return "someone@example.com" }

func TestAPersonChoosingAModelSeesWhatEachCosts(t *testing.T) {
	here, reg, offered := registered(t, registry.Rules{})
	ledger := spend.NewLedger(spend.Policy{Prices: map[string]spend.Price{"platform/qwen": {InputPerMillion: 1, OutputPerMillion: 2}}})
	here.server.UseLedger(ledger)
	reg.OnChange(func(e registry.Effective) {
		ledger.SetPrices(e.Prices)
		m := Models(e.Models)
		offered.Store(&m)
	})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big"}, "input": {"0.25"}, "output": {"1.5"}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"free-ish"}})
	admin.ask(t, "POST", "/admin/models/providers/acme/enable", nil)
	admin.ask(t, "POST", "/admin/models/providers/acme/models/enable", url.Values{"model": {"big"}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models/enable", url.Values{"model": {"free-ish"}})
	page := here.as(t, alice).ask(t, "GET", "/instances/new?harness=fixture", nil).Body.String()
	for _, want := range []string{"platform/qwen · $1 in, $2 out per million tokens", "acme/big · $0.25 in, $1.5 out per million tokens", "acme/free-ish · no price"} {
		if !strings.Contains(page, want) {
			t.Errorf("the chooser does not say %q:\n%s", want, page)
		}
	}
	none := open(t)
	none.server.UseLedger(spend.NewLedger(spend.Policy{}))
	none.signIn(t)
	if strings.Contains(none.ask(t, "GET", "/instances/new?harness=fixture", nil).Body.String(), "no price") {
		t.Error("a deployment with no prices at all says so against every model")
	}
}

func TestAnAdminGoesBackToAnEarlierVersionThroughThePageAndItIsRecordedWithWhatItUndid(t *testing.T) {
	here, reg, offered := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big"}})
	admin.ask(t, "POST", "/admin/models/providers/acme/enable", nil)
	admin.ask(t, "POST", "/admin/models/providers/acme/models/enable", url.Values{"model": {"big"}})
	before := reg.Document().Version
	admin.ask(t, "POST", "/admin/models/providers/acme/models/retire", url.Values{"model": {"big"}})
	if got := *offered.Load(); len(got) != 1 {
		t.Fatalf("retired and still offered: %v", got)
	}
	body := admin.ask(t, "GET", modelsTab("settings"), nil).Body.String()
	if !strings.Contains(body, "Go back") || !strings.Contains(body, "retired model acme/big") {
		t.Errorf("the page does not offer going back, or say what was done after:\n%s", body)
	}
	got := admin.ask(t, "POST", "/admin/models/restore", url.Values{"version": {strconv.FormatInt(before, 10)}})
	if got.Header().Get("Location") != modelsTab("settings") {
		t.Fatalf("%v", got.Header())
	}
	if offered := *offered.Load(); len(offered) != 2 || offered[1] != "acme/big" {
		t.Errorf("after going back: %v", offered)
	}
	var restored []changelog.Event
	for _, event := range written(t, here.log) {
		if event.Kind == changelog.RegistryRestored {
			restored = append(restored, event)
		}
	}
	if len(restored) != 1 || restored[0].What["undid"] != "brought back model acme/big" {
		t.Errorf("the change record has %+v", restored)
	}
	if refused := admin.ask(t, "POST", "/admin/models/restore", url.Values{"version": {"9999"}}); !strings.Contains(refused.Header().Get("Location"), "refused=") {
		t.Error("a version that was never kept was restored")
	}
}
