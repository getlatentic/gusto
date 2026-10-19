package web

import (
	"github.com/getlatentic/gusto/internal/instance"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The chat page offers the agent and the model its message will go to, with the
// first agent and its first model chosen when nothing was used before.
func TestTheChatPageOffersAnAgentAndAModel(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	page := site.ask(t, "GET", "/chat", nil).Body.String()
	for _, want := range []string{`name="harness"`, `name="model"`, `name="harness" value="first"`, `action="/chat"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the chat page lacks %s", want)
		}
	}
}

// A message goes to the agent chosen and opens a conversation there; the next
// chat with the same agent carries on in the same agent rather than starting another.
func TestAChatGoesToTheChosenAgentAndCarriesOn(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	first := site.ask(t, "POST", "/chat", url.Values{"harness": {"second"}, "model": {"platform/test-model"}, "said": {"hello"}})
	where := first.Header().Get("Location")
	if !strings.Contains(where, "/threads/") {
		t.Fatalf("a chat answered %d to %q", first.Code, where)
	}
	again := site.ask(t, "POST", "/chat", url.Values{"harness": {"second"}, "said": {"and again"}}).Header().Get("Location")
	if strings.Split(where, "/threads/")[0] != strings.Split(again, "/threads/")[0] || where[:strings.Index(where, "#")] == again[:strings.Index(again, "#")] {
		t.Errorf("the second chat went to %q, not a new conversation on the same agent as %q", again, where)
	}
	if page := site.ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, `name="harness" value="second"`) {
		t.Error("the chat page does not offer the agent used last")
	}
}

// An agent or model that cannot be used is turned away to the chat page, with the reason.
func TestAChatOnAModelTheAgentCannotUseIsTurnedAway(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	answer := site.ask(t, "POST", "/chat", url.Values{"harness": {"first"}, "model": {"nobody/model"}, "said": {"hello"}})
	if !strings.HasPrefix(answer.Header().Get("Location"), "/chat") {
		t.Errorf("a refused chat went to %q", answer.Header().Get("Location"))
	}
}

// The model is changed from the conversation, and the page comes back to it.
func TestTheModelIsChangedFromTheConversation(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	instance := where[:strings.Index(where, "/threads/")]
	answer := site.ask(t, "POST", instance+"/model", url.Values{"model": {"platform/test-model"}, "back": {where}})
	if answer.Header().Get("Location") != where {
		t.Errorf("changing the model went to %q, not back to %q", answer.Header().Get("Location"), where)
	}
	away := site.ask(t, "POST", instance+"/model", url.Values{"model": {"platform/test-model"}, "back": {"//evil.example/"}})
	if away.Header().Get("Location") != instance {
		t.Errorf("a foreign address was followed: %q", away.Header().Get("Location"))
	}
}

// A chat started for a folder on the person's computer goes to the agent kept for that folder, and a chat for no folder never does.
func TestAChatForAFolderHasAnAgentOfItsOwn(t *testing.T) {
	site := twoEngines(t)
	site.signIn(t)
	key := "0123456789abcdef"
	first := site.ask(t, "POST", "/chat", url.Values{"harness": {"first"}, "said": {"in the folder"}, "folder": {key}, "folder_name": {"thesis"}}).Header().Get("Location")
	plain := site.ask(t, "POST", "/chat", url.Values{"harness": {"first"}, "said": {"in no folder"}}).Header().Get("Location")
	again := site.ask(t, "POST", "/chat", url.Values{"harness": {"first"}, "said": {"in the folder again"}, "folder": {key}, "folder_name": {"thesis"}}).Header().Get("Location")
	agent := func(where string) string { return where[:strings.Index(where, "/threads/")] }
	if agent(first) == agent(plain) {
		t.Error("a chat for no folder went to the agent kept for a folder")
	}
	if agent(first) != agent(again) {
		t.Errorf("a second chat for the folder went to %s, not %s", agent(again), agent(first))
	}
	var listed instance.Instance
	for _, each := range site.server.instances.List() {
		if each.Folder == key {
			listed = each
		}
	}
	if listed.ID == "" || listed.Title != "thesis" {
		t.Errorf("the agent kept for the folder is not held with it: %+v", listed)
	}
}

// A key the app did not make is no folder.
func TestAFolderKeyIsOnlyWhatTheAppMakes(t *testing.T) {
	for _, bad := range []string{"", "short", "0123456789ABCDEF", "0123456789abcdeg", "0123456789abcdef0"} {
		request := httptest.NewRequest("POST", "/chat", strings.NewReader(url.Values{"folder": {bad}}.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if got := folderOf(request); got.Key != "" {
			t.Errorf("%q was taken for a folder", bad)
		}
	}
}

// A preset is saved from the chat, shows on the roles page and in the chat's role menu, and is the person's own.
func TestAPresetIsSavedFromTheChatAndOffered(t *testing.T) {
	site := open(t)
	mine, theirs := site.as(t, alice), site.as(t, bob)
	saved := mine.ask(t, "POST", "/presets", url.Values{"name": {"Reviewer"}, "harness": {"fixture"}, "model": {"platform/test-model"}, "role": {"engineering"}})
	where := saved.Header().Get("Location")
	if saved.Code != http.StatusSeeOther || !strings.HasPrefix(where, "/chat?preset=") {
		t.Fatalf("saving answered %d to %q", saved.Code, where)
	}
	if page := mine.ask(t, "GET", where, nil).Body.String(); !strings.Contains(page, "Reviewer") || !strings.Contains(page, `value="p:`) {
		t.Errorf("the chat does not offer the preset and open on it:\n%s", page)
	}
	if roles := mine.ask(t, "GET", "/new", nil).Body.String(); !strings.Contains(roles, "Reviewer") || !strings.Contains(roles, "Use in a new chat") {
		t.Error("the roles page does not list the preset")
	}
	if other := theirs.ask(t, "GET", "/new", nil).Body.String(); strings.Contains(other, "Reviewer") {
		t.Error("somebody else sees the preset")
	}
	id := strings.TrimPrefix(where, "/chat?preset=")
	if gone := theirs.ask(t, "POST", "/presets/"+id+"/delete", nil); gone.Code != http.StatusSeeOther {
		t.Fatalf("deleting answered %d", gone.Code)
	}
	if page := mine.ask(t, "GET", "/new", nil).Body.String(); !strings.Contains(page, "Reviewer") {
		t.Error("somebody else removed the preset")
	}
	mine.ask(t, "POST", "/presets/"+id+"/delete", nil)
	if page := mine.ask(t, "GET", "/new", nil).Body.String(); strings.Contains(page, "Reviewer") {
		t.Error("the preset was not removed")
	}
}

func TestAChatWithARoleStartsAnAgentThatHasItsSkills(t *testing.T) {
	site := open(t)
	site.signIn(t)
	started := site.ask(t, "POST", "/chat", url.Values{"said": {"fix it"}, "harness": {"fixture"}, "role": {"engineering"}})
	where := started.Header().Get("Location")
	if started.Code != http.StatusSeeOther || !strings.Contains(where, "/threads/") {
		t.Fatalf("starting a chat with a role answered %d to %q", started.Code, where)
	}
	instance := where[:strings.Index(where, "/threads/")]
	if page := site.ask(t, "GET", instance, nil).Body.String(); !strings.Contains(page, "Software engineer") {
		t.Errorf("the agent is not the role's:\n%s", page)
	}
	again := site.ask(t, "POST", "/chat", url.Values{"said": {"and again"}, "harness": {"fixture"}, "role": {"engineering"}})
	if got := again.Header().Get("Location"); !strings.HasPrefix(got, instance+"/threads/") {
		t.Errorf("a second chat with the role made another agent: %q, not under %q", got, instance)
	}
}
