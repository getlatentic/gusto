package web

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// A person who ends a connection and makes it again starts from their default for their agents: what they chose for an agent under
// the old one is gone with it.
func TestAnAppConnectedAgainFollowsThePersonsDefaultAndNotWhatTheyChoseBefore(t *testing.T) {
	here, _, app := connected(t)
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	ada := here.as(t, alice)
	signInAt(t, ada, app)
	room, err := here.server.instances.StartRoom("Launch", alice)
	if err != nil {
		t.Fatal(err)
	}
	nova := aCrewAgent(t, here, alice, "Nova", "opencode")
	inRoom, err := here.server.instances.AddAgent(room.ID, instance.Agent{Name: nova.Name, Harness: nova.Harness, Member: nova.ID})
	if err != nil {
		t.Fatal(err)
	}
	turn := func(id string) []string {
		spec, _ := task.Read(placed(t, here, id, tasks.Record{Asked: alice, Agent: inRoom.ID, Instance: room.ID}, "opencode"))
		if spec.Setup == nil {
			return nil
		}
		return spec.Setup.Connectors
	}
	if got := ada.ask(t, "POST", "/members/agents/"+nova.ID+"/apps", url.Values{"app": {"tasks"}, "choice": {"allow"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("choosing answered %d", got.Code)
	}
	if got := turn("allowed"); len(got) != 1 {
		t.Fatalf("the app the person allowed was not given: %v", got)
	}
	ada.ask(t, "POST", "/connections/tasks/disconnect", nil)
	signInAt(t, ada, app)
	if got := turn("again"); got != nil {
		t.Errorf("an app connected again was given by a choice made under the connection that ended, with the default off: %v", got)
	}
}
