package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/instance"
)

func TestANewChatRunsOnTheLastChoiceThenTheOrganizationsDefaultThenTheFirstThatCanRun(t *testing.T) {
	picks := []pick{
		{Entry: catalog.Entry{Name: "first"}, Models: []string{"m1", "m2"}},
		{Entry: catalog.Entry{Name: "second"}, Models: []string{"m3", "m4"}},
	}
	for name, c := range map[string]struct {
		recent        []yours
		org           chatDefault
		harness, mode string
	}{
		"nothing chosen":                {nil, chatDefault{}, "first", "m1"},
		"the organization's default":    {nil, chatDefault{"second", "m4"}, "second", "m4"},
		"a default model that is gone":  {nil, chatDefault{"second", "gone"}, "second", "m3"},
		"a default engine that is gone": {nil, chatDefault{"gone", "m4"}, "first", "m1"},
		"the person's own last choice":  {[]yours{{agent: agent{Instance: instance.Instance{Harness: "first", Model: "m2"}}}}, chatDefault{"second", "m4"}, "first", "m2"},
	} {
		harness, model := defaultPick(picks, c.recent, c.org)
		if harness != c.harness || model != c.mode {
			t.Errorf("%s: %s %s, want %s %s", name, harness, model, c.harness, c.mode)
		}
	}
}

func TestAnAdminSetsWhatANewChatRunsOnAndTheChangeIsRecorded(t *testing.T) {
	here, log, path := budgeted(t)
	if choices := here.server.defaultChoices(httptest.NewRequest("GET", "/admin", nil)); !slices.ContainsFunc(choices, func(c defaultChoice) bool { return c.Value == "fixture|platform/test-model" }) {
		t.Fatalf("no default for a new chat is offered: %+v", choices)
	}

	set := here.ask(t, "POST", "/admin/chat-default", url.Values{"default": {"fixture|platform/test-model"}})
	if set.Code != http.StatusSeeOther || set.Header().Get("Location") != "/admin" {
		t.Fatalf("setting the default answered %d %s", set.Code, set.Header().Get("Location"))
	}
	kept, _ := os.ReadFile(path)
	if !strings.Contains(string(kept), `"defaultHarness": "fixture"`) && !strings.Contains(string(kept), `"defaultHarness":"fixture"`) {
		t.Errorf("the default was not kept: %s", kept)
	}
	if n := len(ofKind(written(t, log), changelog.ChatDefaultSet)); n != 1 {
		t.Errorf("%d changes were recorded, want 1", n)
	}

	chat := here.ask(t, "GET", "/chat", nil).Body.String()
	if !regexp.MustCompile(`name="harness" value="fixture"`).MatchString(chat) || !strings.Contains(chat, `name="model" value="platform/test-model"`) {
		t.Errorf("a new chat does not start on the organization's default")
	}

	refused := here.ask(t, "POST", "/admin/chat-default", url.Values{"default": {"nothing|that-runs"}})
	if !strings.Contains(refused.Header().Get("Location"), "refused=") {
		t.Errorf("an engine that cannot run was taken: %d %s", refused.Code, refused.Header().Get("Location"))
	}
}

func TestTheNewChatBoxSaysWhatItRunsOnAndKeepsTheChoicesBehindIt(t *testing.T) {
	site := open(t)
	site.signIn(t)
	page := site.ask(t, "GET", "/chat", nil).Body.String()
	for _, want := range []string{`id="run-on"`, `aria-controls="chat-choices"`, `id="chat-choices" hidden`, `id="harness-pick"`, `id="model-pick"`, `id="role-pick"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the new chat lacks %q", want)
		}
	}
	tools := page[strings.Index(page, `id="folder-pick"`):strings.Index(page, `id="chat-choices"`)]
	if strings.Contains(tools, `id="harness-pick"`) || strings.Contains(tools, `id="model-pick"`) {
		t.Error("the engine and model menus are still beside Send")
	}
}

func TestTheOperatorsDefaultIsUsedUntilTheOrganizationChoosesItsOwn(t *testing.T) {
	here, _, _ := budgeted(t)
	here.server.UseChatDefault("fixture", "platform/test-model")
	if got := here.server.orgDefault(); got != (chatDefault{"fixture", "platform/test-model"}) {
		t.Errorf("with no organization default, a new chat runs on %v", got)
	}
	here.ask(t, "POST", "/admin/chat-default", url.Values{"default": {"fixture|platform/test-model"}})
	here.server.UseChatDefault("other", "m")
	if got := here.server.orgDefault(); got.Harness != "fixture" {
		t.Errorf("the organization's own default did not win over the operator's: %v", got)
	}
}
