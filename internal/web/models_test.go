package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/signing"
)

// Two engines, and models that only the first of them can use: what a host has when one
// credential answers one engine.
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

func TestAnAgentIsNotStartedOnAModelItsEngineCannotUse(t *testing.T) {
	site := modelSite(t, Models{"": {"platform/qwen", "anthropic/deepseek"}, "fixture": {"platform/qwen"}, "claude": {}})

	for name, form := range map[string]url.Values{
		"an engine no model serves":            {"harness": {"claude"}},
		"an engine with a model it cannot use": {"harness": {"fixture"}, "model": {"anthropic/deepseek"}},
		"a model that is not offered at all":   {"harness": {"fixture"}, "model": {"platform/nothing"}},
	} {
		got := site.ask(t, "POST", "/instances", form)
		if where := got.Header().Get("Location"); !strings.Contains(where, "refused=") || strings.Contains(where, "/threads/") {
			t.Errorf("%s was started: %d to %q", name, got.Code, where)
		}
	}
	if len(site.server.instances.List()) != 0 {
		t.Error("an agent was made for a pair that cannot work")
	}
	if got := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "model": {"platform/qwen"}}); !strings.Contains(got.Header().Get("Location"), "/threads/") {
		t.Errorf("a pair that works is started: %q", got.Header().Get("Location"))
	}
	if got := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}}); !strings.Contains(got.Header().Get("Location"), "/threads/") {
		t.Errorf("no model named is the first it can use: %q", got.Header().Get("Location"))
	}
}

func TestAnEngineNoModelServesIsNotOfferedAndAgentsStartOnTheOneThatWorks(t *testing.T) {
	site := modelSite(t, Models{"": {"platform/qwen"}, "fixture": {"platform/qwen"}, "claude": {}})

	page := site.ask(t, "GET", "/new/blank", nil).Body.String()
	if strings.Contains(page, `value="claude"`) {
		t.Errorf("an engine nothing serves is offered on the form:\n%s", page)
	}
	if got := site.ask(t, "GET", "/instances/new?harness=claude", nil); !strings.Contains(got.Header().Get("Location"), "refused=") {
		t.Errorf("its set-up page opened: %d", got.Code)
	}
	started := site.ask(t, "POST", "/instances", url.Values{})
	instance := strings.TrimPrefix(started.Header().Get("Location"), "/instances/")
	instance = instance[:strings.Index(instance, "/")]
	if held, _ := site.server.instances.Get(instance); held.Harness != "fixture" || held.Model != "platform/qwen" {
		t.Errorf("a blank agent is started on what works: %+v", held)
	}
	// And a deployment that names no model at all leaves an engine to its own default.
	open := modelSite(t, Everything(nil))
	if got := open.ask(t, "POST", "/instances", url.Values{"harness": {"claude"}}); !strings.Contains(got.Header().Get("Location"), "/threads/") {
		t.Errorf("with no models named an engine uses its own: %q", got.Header().Get("Location"))
	}
}

func TestAPersonChangesTheModelOfTheirAgentAmongThoseItsEngineCanUse(t *testing.T) {
	site := modelSite(t, Models{"": {"a/one", "a/two", "b/other"}, "fixture": {"a/one", "a/two"}})
	mine, theirs := site.as(t, alice), site.as(t, bob)
	where, thread := started(t, mine)
	id := strings.TrimPrefix(where, "/instances/")
	model := func() string { held, _ := site.server.instances.Get(id); return held.Model }
	if model() != "a/one" {
		t.Fatalf("started on %q", model())
	}
	if page := mine.ask(t, "GET", where, nil).Body.String(); !strings.Contains(page, `action="/instances/`+id+`/model"`) || !strings.Contains(page, `value="a/two"`) || strings.Contains(page, `value="b/other"`) {
		t.Errorf("the page offers the engine's models and no others:\n%s", page)
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
		"a model the engine cannot use": {"model": {"b/other"}},
		"a model nobody offers":         {"model": {"nope/nope"}},
		"no model":                      {"model": {""}},
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
	site := modelSite(t, Models{"": {"a/one"}})
	where := startedInstance(t, site)
	if page := site.ask(t, "GET", where, nil).Body.String(); strings.Contains(page, "/model\"") {
		t.Errorf("a chooser with nothing to choose:\n%s", page)
	}
}
