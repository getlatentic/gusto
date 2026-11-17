package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
)

func TestARoomSaysWhoseEachAgentIs(t *testing.T) {
	work := &roomWork{}
	site, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	mine, his := site.as(t, alice), site.as(t, bob)
	mine.ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}})
	mine.ask(t, "POST", "/rooms/"+id+"/members/role", url.Values{"who": {bob}, "role": {"admin"}})
	atlas := aCrewAgent(t, site, bob, "Atlas", "other")
	if added := his.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {atlas.ID}}); strings.Contains(added.Header().Get("Location"), "refused") {
		t.Fatalf("bob could not add his agent: %s", added.Header().Get("Location"))
	}
	if _, err := site.server.instances.AddAgent(id, instance.Agent{Name: "Relic", Harness: "fixture"}); err != nil {
		t.Fatal(err)
	}
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout and @Atlas, what next?"}})
	work.waitTurns(t, site, 2)

	for _, view := range []struct {
		who          string
		scout, atlas string
	}{
		{alice, "your agent", "Bob's agent"},
		{bob, "Alice's agent", "your agent"},
	} {
		page := site.as(t, view.who).conversation(t, thread)
		owners := map[string]string{}
		for _, each := range page.Members.Agents {
			owners[each.Name] = each.Job + " · " + each.Owner
			if each.Mine != (each.Owner == "" && each.Name != "Relic") {
				t.Errorf("%s's members dialog says %s is theirs: %v", view.who, each.Name, each.Mine)
			}
		}
		label := func(owner string) string {
			if owner == "your agent" {
				return ""
			}
			return owner
		}
		if owners["Scout"] != "does Scout · "+label(view.scout) || owners["Atlas"] != "does Atlas · "+label(view.atlas) {
			t.Errorf("%s's members dialog says %v", view.who, owners)
		}
		for _, each := range page.Turns {
			if want := map[string]string{"Scout": alice, "Atlas": bob}[each.By]; each.Owner != want || each.OwnerName != nameFromAddress(want) {
				t.Errorf("%s's view says %s is %q's (%q)", view.who, each.By, each.Owner, each.OwnerName)
			}
		}
		mentions := map[string]mentionable{}
		for _, each := range page.Mentions {
			mentions[each.Name] = each
		}
		if scout := mentions["Scout"]; scout.Role != "does Scout" || scout.Owner != view.scout || !scout.Agent {
			t.Errorf("%s may mention Scout as %+v", view.who, scout)
		}
		if atlas := mentions["Atlas"]; atlas.Role != "does Atlas" || atlas.Owner != view.atlas {
			t.Errorf("%s may mention Atlas as %+v", view.who, atlas)
		}
		if relic := mentions["Relic"]; relic.Role != "Fixture" || relic.Owner != "" || !relic.Agent {
			t.Errorf("%s's view gives an owner to an agent the team does not keep: %+v", view.who, relic)
		}
	}
}
