package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/harness"
)

// Two engines that can both run anything.
func claudeAndFixture() []catalog.Entry {
	ready := func(name string) catalog.Entry {
		return catalog.Entry{Name: name, Program: "sh", Needs: harness.NeedsNothing, Ready: true, Skills: true, MCP: true}
	}
	return []catalog.Entry{ready("claude"), ready("fixture")}
}

func modelSite(t *testing.T, models Models) *site {
	t.Helper()
	site := buildOffering(t, t.TempDir(), signing.Keyring{}, claudeAndFixture(), models)
	site.signIn(t)
	return site
}

// Every engine is offered every model: which pairs work well is the person's to find out. A model that is not offered
// is refused, however the request was made.
func TestEveryEngineStartsOnEveryModelOfferedAndOnNoOther(t *testing.T) {
	site := modelSite(t, Models{"platform/qwen", "anthropic/deepseek"})
	for _, engine := range []string{"claude", "fixture"} {
		for _, model := range []string{"platform/qwen", "anthropic/deepseek"} {
			got := site.ask(t, "POST", "/instances", url.Values{"harness": {engine}, "model": {model}})
			if !strings.Contains(got.Header().Get("Location"), "/threads/") {
				t.Errorf("%s on %s was refused: %q", engine, model, got.Header().Get("Location"))
			}
		}
	}
	if page := site.ask(t, "GET", "/new/blank", nil).Body.String(); !strings.Contains(page, `value="claude"`) || !strings.Contains(page, `value="fixture"`) {
		t.Errorf("the form offers every engine:\n%s", page)
	}
	before := len(site.server.instances.List())
	if got := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "model": {"platform/nothing"}}); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("a model that is not offered was started: %q", got.Header().Get("Location"))
	}
	if len(site.server.instances.List()) != before {
		t.Error("an agent was made on a model nobody offers")
	}
	started := site.ask(t, "POST", "/instances", url.Values{"harness": {"claude"}})
	id := strings.TrimPrefix(started.Header().Get("Location"), "/instances/")
	id = id[:strings.Index(id, "/")]
	if held, _ := site.server.instances.Get(id); held.Model != "platform/qwen" {
		t.Errorf("no model named is the first offered: %+v", held)
	}
	open := modelSite(t, Models(nil))
	if got := open.ask(t, "POST", "/instances", url.Values{"harness": {"claude"}}); !strings.Contains(got.Header().Get("Location"), "/threads/") {
		t.Errorf("with no models named an engine uses its own: %q", got.Header().Get("Location"))
	}
}

func TestAPersonChangesTheModelOfTheirAgentAmongThoseOffered(t *testing.T) {
	site := modelSite(t, Models{"a/one", "a/two"})
	mine, theirs := site.as(t, alice), site.as(t, bob)
	where, thread := started(t, mine)
	id := strings.TrimPrefix(where, "/instances/")
	model := func() string { held, _ := site.server.instances.Get(id); return held.Model }
	if model() != "a/one" {
		t.Fatalf("started on %q", model())
	}
	if page := mine.ask(t, "GET", where, nil).Body.String(); !strings.Contains(page, `action="/instances/`+id+`/model"`) || !strings.Contains(page, `value="a/two"`) {
		t.Errorf("the page offers the models offered:\n%s", page)
	}

	if got := mine.ask(t, "POST", where+"/model", url.Values{"model": {"a/two"}}); got.Code != http.StatusSeeOther || strings.Contains(got.Header().Get("Location"), "refused") || model() != "a/two" {
		t.Fatalf("the change was refused: %d %q, on %q", got.Code, got.Header().Get("Location"), model())
	}
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"and now?"}})
	turns := site.index.List()
	if len(turns) != 1 || turns[0].Model != "a/two" {
		t.Errorf("the next turn's record names the model it ran on: %+v", turns)
	}

	for name, form := range map[string]url.Values{
		"a model nobody offers": {"model": {"nope/nope"}},
		"no model":              {"model": {""}},
	} {
		if got := mine.ask(t, "POST", where+"/model", form); !strings.Contains(got.Header().Get("Location"), "refused=") || model() != "a/two" {
			t.Errorf("%s: %q, and it is on %q", name, got.Header().Get("Location"), model())
		}
	}
	if got := theirs.ask(t, "POST", where+"/model", url.Values{"model": {"a/one"}}); got.Code != http.StatusNotFound || model() != "a/two" {
		t.Errorf("somebody else changed the model: %d, on %q", got.Code, model())
	}
}

func TestAnAgentWithOneModelToUseIsShownItAndNoChooser(t *testing.T) {
	site := modelSite(t, Models{"a/one"})
	where := startedInstance(t, site)
	if page := site.ask(t, "GET", where, nil).Body.String(); strings.Contains(page, "/model\"") {
		t.Errorf("a chooser with nothing to choose:\n%s", page)
	}
}
