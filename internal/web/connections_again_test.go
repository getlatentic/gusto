package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// A sign-in the app no longer takes is not shown as connected and not given to a task: Connections, the admin's page and the
// agent's Apps tab say it needs signing in again, and the agent in a room is told that and not that nothing is disconnected.
func TestASignInThatNeedsMakingAgainSaysSoWhereverAppsAreShown(t *testing.T) {
	here, _, app := connected(t)
	app.GivesNoRefresh()
	app.Lasts(time.Second)
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	ada := here.as(t, alice)
	signInAt(t, ada, app)
	time.Sleep(1100 * time.Millisecond)

	page := ada.ask(t, "GET", "/connections", nil).Body.String()
	if !strings.Contains(page, ">sign in again<") || !strings.Contains(page, `href="/connections/tasks/connect">Sign in again<`) {
		t.Errorf("Connections does not ask the person to sign in again:\n%s", page)
	}
	if strings.Contains(page, `action="/connections/tasks/agents"`) {
		t.Error("Connections offers a default for agents to an app that does not work")
	}
	if !strings.Contains(page, `action="/connections/tasks/disconnect"`) {
		t.Error("the person cannot end a sign-in that needs making again")
	}
	if admin := here.ask(t, "GET", "/admin/apps?tab=apps", nil).Body.String(); !strings.Contains(admin, "1 person needs to sign in again") {
		t.Errorf("the admin's page does not say who needs to sign in again:\n%s", admin)
	}
	if spec, _ := task.Read(placed(t, here, "own", tasks.Record{Asked: alice}, "opencode")); spec.Setup != nil {
		t.Errorf("a task was given an app whose sign-in no longer works: %+v", spec.Setup)
	}

	room, err := here.server.instances.StartRoom("Launch", alice, "")
	if err != nil {
		t.Fatal(err)
	}
	nova := aCrewAgent(t, here, alice, "Nova", "opencode")
	inRoom, err := here.server.instances.AddAgent(room.ID, instance.Agent{Name: nova.Name, Harness: nova.Harness, Member: nova.ID})
	if err != nil {
		t.Fatal(err)
	}
	if tab := ada.ask(t, "GET", "/members/agents/"+nova.ID+"?tab=apps", nil).Body.String(); !strings.Contains(tab, ">sign in again<") || strings.Contains(tab, `name="choice"`) {
		t.Errorf("the agent's Apps tab does not ask for a sign-in, or offers a choice for an app that does not work:\n%s", tab)
	}
	placed(t, here, "room", tasks.Record{Asked: alice, Agent: inRoom.ID, Instance: room.ID}, "opencode")
	brief, _ := task.Read(task.Dir(filepath.Join(here.index.Root(), "room")))
	if !strings.Contains(brief.Brief, "sign-in to Tasks needs signing in again") || strings.Contains(brief.Brief, "Nothing is disconnected, so never suggest reconnecting") {
		t.Errorf("the agent is not told the sign-in needs making again:\n%s", brief.Brief)
	}

	here.ask(t, "POST", "/admin/connectors/tasks/off", nil)
	if got := ada.ask(t, "POST", "/connections/tasks/disconnect", nil).Code; got != http.StatusNotFound {
		t.Errorf("disconnecting an app that is off answered %d, as connecting it does not", got)
	}
}
