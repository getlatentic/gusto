package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// The data of the pages a person starts an agent from, in the shapes web/src/pages/setups/types.ts names.

type setupsJSON struct {
	Setups  []setupRow
	Keeps   bool
	Refused string
}

type setupJSON struct {
	Set       skillSetView
	Servers   []string
	Offers    bool
	Uses      []struct{ Name string }
	StartWith []setupStarter
	Refused   string
}

type presetsJSON struct {
	Roles   []role
	Fields  []string
	Presets []presetRow
	Refused string
}

type templateJSON struct {
	Template   templateView
	Harnesses  []templateRunner
	InBuilding bool
	Refused    string
}

type blankJSON struct {
	Engines []blankEngine
	Setups  []string
	Refused string
}

type setUpJSON struct {
	Harness setUpHarness
	Models  []option
	Setups  []option
	Chosen  string
	Then    string
	Refused string
}

func presetsOf(t *testing.T, s *site) presetsJSON {
	t.Helper()
	var page presetsJSON
	s.data(t, "/presets", &page)
	return page
}

func setUpOf(t *testing.T, s *site, query string) setUpJSON {
	t.Helper()
	var page setUpJSON
	s.data(t, "/instances/new"+query, &page)
	return page
}

func blankOf(t *testing.T, s *site) blankJSON {
	t.Helper()
	var page blankJSON
	s.data(t, "/new/blank", &page)
	return page
}

func (page presetsJSON) role(t *testing.T, name string) role {
	t.Helper()
	at := slices.IndexFunc(page.Roles, func(each role) bool { return each.Name == name })
	if at < 0 {
		t.Fatalf("the presets page does not offer %s: %+v", name, page.Roles)
	}
	return page.Roles[at]
}

func (page presetsJSON) saved(name string) bool {
	return slices.ContainsFunc(page.Presets, func(each presetRow) bool { return each.Name == name })
}

func (page setUpJSON) offers(model string) bool {
	return slices.ContainsFunc(page.Models, func(each option) bool { return each.Value == model })
}

func (page setUpJSON) says(label string) bool {
	return slices.ContainsFunc(page.Models, func(each option) bool { return each.Label == label })
}

func (page blankJSON) engine(name string) bool {
	return slices.ContainsFunc(page.Engines, func(each blankEngine) bool { return each.Name == name })
}

// The pages a person starts an agent from are the app's page, behind their sign-in, and so is their data.
func TestThePagesAnAgentStartsFromAreTheAppBehindTheSignIn(t *testing.T) {
	here := open(t)
	stranger := &site{handler: here.handler, index: here.index, server: here.server}
	here.signIn(t)
	for _, path := range []string{"/setups", "/presets", "/templates/health", "/new/blank", "/instances/new?harness=fixture"} {
		page := here.ask(t, "GET", path, nil)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<div id="root">`) {
			t.Errorf("%s is not the app: %d", path, page.Code)
		}
		if got := stranger.ask(t, "GET", path, nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/login" {
			t.Errorf("a stranger asking for %s got %d to %q", path, got.Code, got.Header().Get("Location"))
		}
		if got := stranger.ask(t, "GET", "/app/api"+path, nil); got.Code != http.StatusUnauthorized {
			t.Errorf("a stranger asking for the data of %s got %d", path, got.Code)
		}
	}
	for _, path := range []string{"/templates/nothing", "/setups/nothing", "/app/api/templates/nothing", "/app/api/setups/nothing"} {
		if got := here.ask(t, "GET", path, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s, which is not there, answered %d", path, got.Code)
		}
	}
}

// A harness this deployment cannot start sends the person back to the presets, saying why.
func TestSettingUpAnAgentThatCannotStartSendsThePersonBack(t *testing.T) {
	site := open(t)
	site.signIn(t)
	got := site.ask(t, "GET", "/instances/new?harness=nothing", nil)
	if where := got.Header().Get("Location"); got.Code != http.StatusSeeOther || !strings.HasPrefix(where, "/presets?refused=") {
		t.Fatalf("setting up an agent that cannot start answered %d to %q", got.Code, where)
	}
	if data := site.ask(t, "GET", "/app/api/instances/new?harness=nothing", nil); data.Code != http.StatusNotFound {
		t.Errorf("its data answered %d", data.Code)
	}
}

// The skill sets list says what each holds, and a server that keeps none says nothing of them.
func TestTheSkillSetsListSaysWhatEachHolds(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}, "server": {"memory"}})
	site.ask(t, "POST", "/admin/skill-sets/reviewer/skills", url.Values{"skill": {"review"}, "description": {"when"}, "body": {"do"}})
	var list setupsJSON
	if site.data(t, "/setups", &list); !list.Keeps || len(list.Setups) != 0 {
		t.Errorf("a skill set that is off is listed: %+v", list)
	}
	site.ask(t, "POST", "/admin/skill-sets/reviewer/enable", nil)
	site.data(t, "/setups", &list)
	if len(list.Setups) != 1 || list.Setups[0].Name != "reviewer" || list.Setups[0].Skills != 1 || !slices.Equal(list.Setups[0].Servers, []string{"memory"}) {
		t.Errorf("the list does not say what the skill set holds: %+v", list.Setups)
	}
	var detail setupJSON
	site.data(t, "/setups/reviewer", &detail)
	if !detail.Offers || !slices.Equal(detail.Servers, []string{"memory"}) || detail.Set.Skills[0].Instructions != "<p>do</p>\n" {
		t.Errorf("the skill set's page does not show its servers and its instructions as HTML: %+v", detail)
	}
	at := slices.IndexFunc(detail.StartWith, func(each setupStarter) bool { return each.Name == "fixture" })
	if at < 0 || detail.StartWith[at].Href != "/instances/new?harness=fixture&setup=reviewer" || detail.StartWith[at].Unfit != "" {
		t.Errorf("the skill set's page does not start an agent with it: %+v", detail.StartWith)
	}
}
