package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAnAgentIsMadeInMembersAndStartsPrivate(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine, his := site.as(t, alice), site.as(t, bob)
	made := mine.ask(t, "POST", "/members/agents", url.Values{"name": {"Atlas"}, "specialty": {"Research"}, "harness": {"other"}, "model": {"platform/test-model"}})
	where := made.Header().Get("Location")
	if made.Code != http.StatusSeeOther || !strings.HasPrefix(where, "/members/agents/") {
		t.Fatalf("making an agent gave %d to %s", made.Code, where)
	}
	member, _ := site.server.crew.Get(strings.TrimPrefix(where, "/members/agents/"))
	if member.Name != "Atlas" || member.Harness != "other" || member.Owner != alice || len(member.Access) != 0 {
		t.Fatalf("made %+v", member)
	}
	if page := his.ask(t, "GET", "/members", nil).Body.String(); !strings.Contains(page, "Atlas") {
		t.Error("the members list does not show the workspace's agent to everybody")
	}
	if got := his.ask(t, "POST", where, url.Values{"name": {"Mine now"}}); got.Code != http.StatusForbidden {
		t.Errorf("somebody else changed the agent: %d", got.Code)
	}
	if got := his.ask(t, "POST", where+"/message", nil); got.Code != http.StatusForbidden {
		t.Errorf("somebody without access messaged the agent: %d", got.Code)
	}
	for _, refused := range []url.Values{
		{"name": {"Atlas"}, "harness": {"other"}},
		{"name": {"Nobody"}, "harness": {"not-here"}},
		{"name": {"Pricey"}, "harness": {"other"}, "model": {"acme/not-offered"}},
	} {
		if got := mine.ask(t, "POST", "/members/agents", refused); !strings.Contains(got.Header().Get("Location"), "refused") {
			t.Errorf("%v was not refused", refused)
		}
	}
}

func TestSharingAnAgentLetsAnotherPersonMessageIt(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine, his := site.as(t, alice), site.as(t, bob)
	where := "/members/agents/" + member.ID
	mine.ask(t, "POST", where+"/access", url.Values{"who": {"Bob@Example.com"}})
	if shared, _ := site.server.crew.Get(member.ID); !shared.MayUse(bob) {
		t.Fatalf("bob was not given access: %+v", shared.Access)
	}
	opened := his.ask(t, "POST", where+"/message", nil)
	if opened.Code != http.StatusSeeOther {
		t.Fatalf("messaging gave %d", opened.Code)
	}
	again := his.ask(t, "POST", where+"/message", nil)
	if again.Header().Get("Location") != opened.Header().Get("Location") {
		t.Errorf("messaging twice opened %s and %s", opened.Header().Get("Location"), again.Header().Get("Location"))
	}
	direct, found := site.server.directRoom(bob, member.ID)
	if !found || !direct.Direct || direct.RoleOf(alice) != "" {
		t.Fatalf("the direct room is %+v", direct)
	}
	page := his.ask(t, "GET", "/chat", nil).Body.String()
	if !strings.Contains(between(page, `aria-label="Direct messages"`, "</section>"), "Atlas") {
		t.Error("the side list does not carry the direct room under Direct messages")
	}
	mine.ask(t, "POST", where+"/access", url.Values{"who": {bob}, "action": {"remove"}})
	if got := his.ask(t, "POST", where+"/message", nil); got.Code != http.StatusForbidden {
		t.Errorf("bob still messaged the agent after losing access: %d", got.Code)
	}
}

func TestRemovingAnAgentTakesItOutOfItsRooms(t *testing.T) {
	work := &roomWork{}
	here, id, _ := aRoom(t, work, [2]string{"Scout", "fixture"})
	held, _ := here.server.instances.Get(id)
	member := held.Agents[0].Member
	if got := here.as(t, bob).ask(t, "POST", "/members/agents/"+member+"/remove", nil); got.Code != http.StatusForbidden {
		t.Errorf("somebody else removed the agent: %d", got.Code)
	}
	here.as(t, alice).ask(t, "POST", "/members/agents/"+member+"/remove", nil)
	if _, found := here.server.crew.Get(member); found {
		t.Error("the agent is still kept")
	}
	if held, _ := here.server.instances.Get(id); len(held.Agents) != 0 {
		t.Errorf("the room still holds %+v", held.Agents)
	}
}

func TestAnAgentsPageShowsItsProfileAccessAndSkills(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine := site.as(t, alice)
	where := "/members/agents/" + member.ID
	mine.ask(t, "POST", where, url.Values{"tab": {"profile"}, "persona": {"You are careful."}, "specialty": {"Research"}})
	for tab, want := range map[string]string{"profile": "You are careful.", "access": "Let somebody work with it", "skills": "No skills given"} {
		if page := mine.ask(t, "GET", where+"?tab="+tab, nil).Body.String(); !strings.Contains(page, want) {
			t.Errorf("the %s tab does not show %q", tab, want)
		}
	}
	if page := site.as(t, bob).ask(t, "GET", where, nil).Body.String(); strings.Contains(page, `name="persona"`) {
		t.Error("somebody else is offered the agent to edit")
	}
	if got := mine.ask(t, "GET", "/members/agents/000000000000", nil); got.Code != http.StatusNotFound {
		t.Errorf("a missing agent answered %d", got.Code)
	}
	if page := mine.ask(t, "GET", "/members/people/"+url.PathEscape(alice), nil).Body.String(); !strings.Contains(page, "Agents they made") || !strings.Contains(page, "Atlas") {
		t.Error("a person's page does not list the agents they made")
	}
}

func TestADirectRoomStillOpensAfterItsAgentIsRemoved(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine := site.as(t, alice)
	opened := mine.ask(t, "POST", "/members/agents/"+member.ID+"/message", nil).Header().Get("Location")
	if page := mine.ask(t, "GET", opened, nil).Body.String(); !strings.Contains(page, "Your own conversation with Atlas") {
		t.Fatal("the direct room does not say whose it is")
	}
	mine.ask(t, "POST", "/members/agents/"+member.ID+"/remove", nil)
	if got := mine.ask(t, "GET", opened, nil); got.Code != http.StatusOK {
		t.Errorf("the direct room answered %d once its agent was removed", got.Code)
	}
}
