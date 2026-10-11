package web

import (
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/instance"
)

func TestANewChatRunsOnTheLastChoiceThenTheOrganisationsDefaultThenTheFirstThatCanRun(t *testing.T) {
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
		"the organisation's default":    {nil, chatDefault{"second", "m4"}, "second", "m4"},
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
	overview := here.ask(t, "GET", "/admin", nil).Body.String()
	if !strings.Contains(overview, `action="/admin/chat-default"`) || !strings.Contains(overview, `value="fixture|platform/test-model"`) {
		i := strings.Index(overview, "New chats")
		t.Fatalf("the overview offers no default for a new chat: %s", overview[max(i, 0):min(max(i, 0)+600, len(overview))])
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
		t.Errorf("a new chat does not start on the organisation's default")
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
	tools := page[strings.Index(page, `class="tools"`):strings.Index(page, `id="chat-choices"`)]
	if strings.Contains(tools, `id="harness-pick"`) || strings.Contains(tools, `id="model-pick"`) {
		t.Error("the engine and model menus are still beside Send")
	}
}
