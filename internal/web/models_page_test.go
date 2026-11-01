package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/registry"
)

func TestTheModelsPageOpensOnTheTableWithTheFormsFolded(t *testing.T) {
	here, _, _ := registered(t, registry.Rules{})
	admin := here.as(t, boss)
	admin.ask(t, "POST", "/admin/models/providers", url.Values{"name": {"acme"}, "address": {"https://api.theirs.example/v1"}, "source": {"stored"}, "key": {theirKey}})
	admin.ask(t, "POST", "/admin/models/providers/acme/models", url.Values{"model": {"big-1"}, "input": {"0.5"}, "output": {"1.5"}})
	admin.ask(t, "POST", "/admin/models/access", url.Values{"email": {bob2()}, "model": {"platform/qwen"}})

	page := admin.ask(t, "GET", "/admin/models", nil).Body.String()
	for _, want := range []string{"Where models answer, what they cost, and who may use them", `data-open="add-provider"`,
		`<td class="whitespace-nowrap" title="platform/qwen">Qwen</td>`, "$1.00", "$2.00", "$0.50", "$1.50",
		"everyone but " + here.server.personName(bob2()), "Who may use which models"} {
		if !strings.Contains(page, want) {
			t.Errorf("the models page lacks %q", want)
		}
	}
	table, folded, form := strings.Index(page, "<table"), strings.Index(page, "<details"), strings.Index(page, `action="/admin/models/providers"`)
	if table < 0 || folded < table || form < strings.Index(page, `<dialog`) {
		t.Errorf("the page does not open on the table with the provider's forms folded and the new provider's in a dialog")
	}
}

func TestTheModelsPageSetsWhatANewChatRunsOnAndHowSandboxesReachAModel(t *testing.T) {
	here, _, _ := registered(t, registry.Rules{})
	here.server.UseOrgSettings(filepath.Join(t.TempDir(), "settings.json"))
	admin := here.as(t, boss)
	page := admin.ask(t, "GET", "/admin/models", nil).Body.String()
	for _, want := range []string{"A new chat starts on", "The first agent and model that can run.", `id="chat-default"`, `action="/admin/chat-default"`,
		"Advanced: how sandboxes reach a model", `action="/admin/run-tokens"`, "Use plain tokens"} {
		if !strings.Contains(page, want) {
			t.Errorf("the models page lacks %q", want)
		}
	}
	choice := regexp.MustCompile(`<option value="([^"|]+\|[^"]+)">([^<]+)</option>`).FindStringSubmatch(page)
	if choice == nil {
		t.Fatal("the models page offers no agent and model for a new chat")
	}
	set := admin.ask(t, "POST", "/admin/chat-default", url.Values{"default": {choice[1]}})
	if set.Code != http.StatusSeeOther || set.Header().Get("Location") != "/admin/models" {
		t.Fatalf("setting the default answered %d %s", set.Code, set.Header().Get("Location"))
	}
	if page := admin.ask(t, "GET", "/admin/models", nil).Body.String(); !strings.Contains(page, choice[2]+". A person's own last choice comes first.") {
		t.Errorf("the page does not say a new chat starts on %s", choice[2])
	}
	if refused := admin.ask(t, "POST", "/admin/chat-default", url.Values{"default": {"nothing|that-runs"}}); !strings.HasPrefix(refused.Header().Get("Location"), "/admin/models?refused=") {
		t.Errorf("a refused default went to %s", refused.Header().Get("Location"))
	}

	plain := admin.ask(t, "POST", "/admin/run-tokens", url.Values{"bound": {"no"}})
	if plain.Code != http.StatusSeeOther || plain.Header().Get("Location") != "/admin/models" {
		t.Fatalf("choosing plain tokens answered %d %s", plain.Code, plain.Header().Get("Location"))
	}
	if page := admin.ask(t, "GET", "/admin/models", nil).Body.String(); !strings.Contains(page, "Bind to the sandbox") {
		t.Error("the models page does not offer to bind runs again")
	}
	if refused := admin.ask(t, "POST", "/admin/run-tokens", url.Values{"bound": {"maybe"}}); !strings.HasPrefix(refused.Header().Get("Location"), "/admin/models?refused=") {
		t.Errorf("a refused choice went to %s", refused.Header().Get("Location"))
	}
}
