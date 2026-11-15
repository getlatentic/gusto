package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// membersJSON is a members page's data, as web/src/pages/members/types.ts names it: the list and the dialogs on every page, and
// whoever the page is of.
type membersJSON struct {
	People   []listedPerson
	Agents   []listedAgent
	Keeps    bool
	NewAgent newAgentView
	Invite   *inviteOffer
	Sent     bool
	Refused  string

	Agent  *agentPageView
	Apps   *agentApps
	Skills *agentSkillsView
	Rooms  []roomLink

	Person *listedPerson
	Bio    *personBio
	Made   []listedAgent
}

// members is the data of the members page at path.
func (s *site) members(t *testing.T, path string) membersJSON {
	t.Helper()
	var page membersJSON
	s.data(t, path, &page)
	return page
}

func (page membersJSON) listsAgent(name string) bool {
	return slices.ContainsFunc(page.Agents, func(each listedAgent) bool { return each.Name == name })
}

func (page membersJSON) kind(t *testing.T, name string) kindView {
	t.Helper()
	at := slices.IndexFunc(page.NewAgent.Kinds, func(each kindView) bool { return each.Name == name })
	if at < 0 {
		t.Fatalf("the new agent dialog does not offer %s: %+v", name, page.NewAgent.Kinds)
	}
	return page.NewAgent.Kinds[at]
}

func (page membersJSON) app(t *testing.T, id string) agentApp {
	t.Helper()
	if page.Apps == nil {
		t.Fatal("the agent's page offers no apps")
	}
	at := slices.IndexFunc(page.Apps.Apps, func(each agentApp) bool { return each.ID == id })
	if at < 0 {
		t.Fatalf("the agent's Apps tab does not list %s: %+v", id, page.Apps.Apps)
	}
	return page.Apps.Apps[at]
}

func TestEachMembersPageIsTheAppAndItsDataAt404WhenNobodyIsThere(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine := site.as(t, alice)
	for _, path := range []string{"/members", "/members/agents/" + member.ID, "/members/people/" + url.PathEscape(alice)} {
		if got := mine.ask(t, "GET", path, nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `<div id="root">`) {
			t.Errorf("%s answered %d without the app", path, got.Code)
		}
	}
	for _, path := range []string{"/members/agents/000000000000", "/members/people/" + url.PathEscape("nobody@example.com")} {
		if got := mine.ask(t, "GET", path, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", path, got.Code)
		}
		if got := mine.ask(t, "GET", "/app/api"+path, nil); got.Code != http.StatusNotFound {
			t.Errorf("the data of %s answered %d", path, got.Code)
		}
	}
}

func TestARefusedNewAgentComesBackToTheDialogWithWhy(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine := site.as(t, alice)
	back := mine.ask(t, "POST", "/members/agents", url.Values{"name": {"Pricey"}, "harness": {"other"}, "model": {"acme/not-offered"}}).Header().Get("Location")
	if where, _ := url.Parse(back); where.Path != "/members" || !where.Query().Has("new") {
		t.Fatalf("a refused agent went to %s", back)
	}
	if page := mine.members(t, back); page.Refused != "that model is not one you may use" {
		t.Errorf("the members page gives the refusal as %q", page.Refused)
	}
	if page := mine.members(t, "/members?new=1"); page.Refused != "" {
		t.Errorf("an unsealed address gives a refusal: %q", page.Refused)
	}
}
