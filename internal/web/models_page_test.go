package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/registry"
)

// modelsJSON is the Models page's data, in the server's own types.
type modelsJSON struct {
	View       modelsView
	Refused    string
	Tabs       []tab
	Tab        string
	Pages      pager
	Rows       []modelRow
	Providers  []providerRow
	Provider   *registry.ProviderView
	Models     []registry.ModelView
	Sources    []keySource
	People     []string
	Limits     []registry.Access
	Steps      []registry.Step
	Defaults   []defaultChoice
	DefaultNow string
	RunTokens  *runTokens
}

// modelsAt is the Models page's data at an address of it.
func modelsAt(t *testing.T, here *site, path string) modelsJSON {
	t.Helper()
	var page modelsJSON
	here.data(t, path, &page)
	return page
}

// rowNamed is the Models tab's row of a model, by its full name.
func (m modelsJSON) rowNamed(full string) (modelRow, bool) {
	at := slices.IndexFunc(m.Rows, func(row modelRow) bool { return row.Full == full })
	if at < 0 {
		return modelRow{}, false
	}
	return m.Rows[at], true
}

// providerNamed is the Providers tab's row of a provider, by its name.
func (m modelsJSON) providerNamed(name string) (providerRow, bool) {
	at := slices.IndexFunc(m.Providers, func(row providerRow) bool { return row.Name == name })
	if at < 0 {
		return providerRow{}, false
	}
	return m.Providers[at], true
}

// modelsOf is the full names of the models one provider's page lists.
func (m modelsJSON) modelsOf() []string {
	var names []string
	for _, model := range m.Models {
		names = append(names, model.Full)
	}
	return names
}

func TestTheModelsPageOpensOnTheTableOfModelsWithTheirPricesAndWhoMayUseThem(t *testing.T) {
	here, _, _ := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example/v1"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big-1"}, "input": {"0.5"}, "output": {"1.5"}})
	admin.ask(t, "POST", "/admin/models/access", url.Values{"email": {bob2()}, "model": {"platform/qwen"}})

	page := modelsAt(t, admin, modelsPath)
	if page.Tab != "models" || page.View.Unnamed {
		t.Errorf("the page does not open on the Models tab with a way to add a provider: %q %+v", page.Tab, page.View)
	}
	qwen, held := page.rowNamed("platform/qwen")
	if !held || qwen.Name != "Qwen" || qwen.Input != "$1.00" || qwen.Output != "$2.00" || qwen.Who != "everyone" {
		t.Errorf("qwen's row is %+v", qwen)
	}
	big, held := page.rowNamed("acme/big-1")
	if !held || big.Input != "$0.50" || big.Output != "$1.50" || !big.Off || big.Who != "everyone but "+here.server.personName(bob2()) {
		t.Errorf("acme/big-1's row is %+v", big)
	}
	if !slices.ContainsFunc(page.Tabs, func(each tab) bool { return each.Href == modelsTab("access") && each.Label == "Who may use" }) {
		t.Errorf("the page does not lead to who may use the models: %+v", page.Tabs)
	}
}

func TestTheModelsPageSetsWhatANewChatRunsOnAndHowSandboxesReachAModel(t *testing.T) {
	here, _, _ := registered(t, registry.Rules{})
	here.server.UseOrgSettings(filepath.Join(t.TempDir(), "settings.json"))
	admin := here.as(t, boss)
	page := modelsAt(t, admin, modelsTab("settings"))
	if page.DefaultNow != "The first agent and model that can run" || page.RunTokens == nil || !page.RunTokens.Bound {
		t.Errorf("the settings say %q and %+v", page.DefaultNow, page.RunTokens)
	}
	if len(page.Defaults) == 0 || !strings.Contains(page.Defaults[0].Value, "|") {
		t.Fatalf("the models page offers no agent and model for a new chat: %+v", page.Defaults)
	}
	choice := page.Defaults[0]
	set := admin.ask(t, "POST", "/admin/chat-default", url.Values{"default": {choice.Value}})
	if set.Code != http.StatusSeeOther || set.Header().Get("Location") != modelsTab("settings") {
		t.Fatalf("setting the default answered %d %s", set.Code, set.Header().Get("Location"))
	}
	if page := modelsAt(t, admin, modelsTab("settings")); page.DefaultNow != choice.Label || !page.Defaults[0].Chosen {
		t.Errorf("the page does not say a new chat starts on %s: %q", choice.Label, page.DefaultNow)
	}
	if refused := admin.ask(t, "POST", "/admin/chat-default", url.Values{"default": {"nothing|that-runs"}}); !strings.HasPrefix(refused.Header().Get("Location"), modelsTab("settings")+"&refused=") {
		t.Errorf("a refused default went to %s", refused.Header().Get("Location"))
	}

	plain := admin.ask(t, "POST", "/admin/run-tokens", url.Values{"bound": {"no"}})
	if plain.Code != http.StatusSeeOther || plain.Header().Get("Location") != modelsTab("settings") {
		t.Fatalf("choosing plain tokens answered %d %s", plain.Code, plain.Header().Get("Location"))
	}
	if page := modelsAt(t, admin, modelsTab("settings")); page.RunTokens == nil || page.RunTokens.Bound {
		t.Errorf("the models page does not say runs get plain tokens: %+v", page.RunTokens)
	}
	if refused := admin.ask(t, "POST", "/admin/run-tokens", url.Values{"bound": {"maybe"}}); !strings.HasPrefix(refused.Header().Get("Location"), modelsTab("settings")+"&refused=") {
		t.Errorf("a refused choice went to %s", refused.Header().Get("Location"))
	}
}
