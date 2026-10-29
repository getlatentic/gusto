package web

import (
	"html"
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
		page := html.UnescapeString(site.as(t, view.who).ask(t, "GET", thread, nil).Body.String())
		for _, want := range []string{
			`>does Scout</span><span class="flex-none text-subtle">· ` + view.scout + `<`,
			`>does Atlas</span><span class="flex-none text-subtle">· ` + view.atlas + `<`,
			`data-agent-owner="` + alice + `" data-yours="· your agent" data-theirs="· Alice's agent">· ` + view.scout + `<`,
			`data-agent-owner="` + bob + `" data-yours="· your agent" data-theirs="· Bob's agent">· ` + view.atlas + `<`,
			`"name":"Scout","role":"does Scout","owner":"` + view.scout + `"`,
			`"name":"Atlas","role":"does Atlas","owner":"` + view.atlas + `"`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s's view of the room does not show %q", view.who, want)
			}
		}
		if !strings.Contains(page, `"name":"Relic","role":"Fixture","agent":true`) {
			t.Errorf("%s's view gives an owner to an agent the workspace does not keep", view.who)
		}
	}
}
